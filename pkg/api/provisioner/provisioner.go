package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	appsv1type "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var clientset *kubernetes.Clientset

type NamespaceRequest struct {
	Namespace string `json:"namespace"`
}

type BackendUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type NamespaceResponse struct {
	Name     string `json:"name,omitempty"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Url      string `json:"url,omitempty"`
}

func Routes(cs *kubernetes.Clientset) *chi.Mux {
	clientset = cs

	router := chi.NewRouter()
	router.Post("/saas", CreateSaaS)
	router.Get("/saas", GetSaaS)
	router.Delete("/saas", DeleteSaaS)
	return router
}

// Get all instances of SaaS
func GetSaaS(w http.ResponseWriter, r *http.Request) {

	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var nsResponse []NamespaceResponse

	// Look for manager": "saas" annotation
	for _, val := range namespaces.Items {
		if annotations := val.GetAnnotations(); annotations != nil && annotations["manager"] == "saas" {

			// Populate the return object
			ns := NamespaceResponse{
				Name:   val.Name,
				Status: annotations["status"],
				Error:  annotations["error"],
				Url:    "http://" + val.Name + tld,
			}

			// Get the secret if the SaaS is in 'Completed' state
			if strings.ToLower(annotations["status"]) == "completed" {
				secret, err := clientset.CoreV1().Secrets(val.Name).Get(context.Background(), "backend-creds", metav1.GetOptions{})
				if err != nil {
					fmt.Printf("Error fetching backend-creds %v\n", err)
					continue
				}

				ns.Username = string(secret.Data["username"])
				ns.Password = string(secret.Data["password"])
			}

			// Append the response
			nsResponse = append(nsResponse, ns)
		}
	}

	// If no namespaces, write empty string
	if len(nsResponse) == 0 {
		_, _ = w.Write([]byte(""))
		return
	}

	// Marshal json
	nsResponseJson, err := json.Marshal(nsResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write response
	_, _ = w.Write(nsResponseJson)

}

// Delete an instance of SaaS
func DeleteSaaS(w http.ResponseWriter, r *http.Request) {
	var ns NamespaceRequest

	// Decode request
	err := json.NewDecoder(r.Body).Decode(&ns)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err = clientset.CoreV1().Namespaces().Get(context.Background(), ns.Namespace, metav1.GetOptions{})
	if err != nil {
		http.NotFound(w, r)
		return
	}

	err = clientset.CoreV1().Namespaces().Delete(context.Background(), ns.Namespace, metav1.DeleteOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusAccepted)

}

// Provisions an instance of Order-Meow UI, Backend, and a Database
// Note: the database is not persistent until a PVC is created and mounted at the correct location
func CreateSaaS(w http.ResponseWriter, r *http.Request) {

	var config NamespaceRequest

	// Decode request
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config.Namespace, err = validateNamespace(config.Namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	namespaceClient := clientset.CoreV1().Namespaces()
	namespaceList, _ := namespaceClient.List(context.TODO(), metav1.ListOptions{})

	// Ensure there isn't a namespace already existing with this name
	for _, val := range namespaceList.Items {
		if val.Name == config.Namespace {
			http.Error(w, "namespace already exists", http.StatusConflict)
			return
		}
	}

	// Provision the SaaS (do background work)
	go func() {
		password := generatePassword()

		// Create deployment objects
		postgresDeploy := getPostgresDeploy()
		backendDeploy := getBackendDeploy()
		frontendDeploy := getFrontendDeploy()

		// Create namespace
		namespace, _ := namespaceClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: config.Namespace,
				Annotations: map[string]string{
					"manager": "saas",
				},
			},
		}, metav1.CreateOptions{})

		// Update postgresql deployment
		for i, container := range postgresDeploy.Spec.Template.Spec.Containers {
			if container.Name == "postgresql" {
				for j, env := range postgresDeploy.Spec.Template.Spec.Containers[i].Env {
					if env.Name == "POSTGRES_PASSWORD" {
						postgresDeploy.Spec.Template.Spec.Containers[i].Env[j].Value = password
						break
					}
				}
				break
			}
		}

		// Create postgresql deployment
		deploymentClient := clientset.AppsV1().Deployments(config.Namespace)
		postgresDeploy, err = deploymentClient.Create(context.Background(), postgresDeploy, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Failed to create postgresql deployment in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create postgresql deployment")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created postgresql deployment")

		// Wait on the postgresql deployment to become ready
		err = waitOnDeployment(deploymentClient, postgresDeploy.Name)
		if err != nil {
			fmt.Printf("Postgresql deployment timeout - not ready in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Postgresql deployment not ready")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: postgresql deployment ready")

		// Create postgresql service
		serviceClient := clientset.CoreV1().Services(config.Namespace)
		service := createService("postgresql", "postgresql", 5432)
		service, err = serviceClient.Create(context.Background(), service, metav1.CreateOptions{})

		if err != nil {
			fmt.Printf("Failed to create postgresql service in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create postgresql service")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created postgresql service")

		// Update backend deployment
		for i, container := range backendDeploy.Spec.Template.Spec.Containers {
			if container.Name == "backend" {
				for j, env := range backendDeploy.Spec.Template.Spec.Containers[i].Env {
					if env.Name == "SPRING_DATASOURCE_PASSWORD" {
						backendDeploy.Spec.Template.Spec.Containers[i].Env[j].Value = password
						break
					}
				}
				break
			}
		}

		// Create backend deployment
		backendDeploy, err = deploymentClient.Create(context.Background(), backendDeploy, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Failed to create backend deployment in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend deployment")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created backend deployment")

		// Wait on the backend deployment to become ready
		err = waitOnDeployment(deploymentClient, backendDeploy.Name)
		if err != nil {
			fmt.Printf("Postgresql deployment timeout - not ready in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Backend deployment not ready")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: backend deployment ready")

		// Create backend service
		service = createService("backend", "backend", 8080)
		service, err = serviceClient.Create(context.Background(), service, metav1.CreateOptions{})

		if err != nil {
			fmt.Printf("Failed to create backend service in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend service")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created backend service")

		// Create backend ingress (namespace-backend.tld)
		ingressClient := clientset.NetworkingV1beta1().Ingresses(config.Namespace)
		backendIngress := createIngress("backend", 8080, config.Namespace)

		backendIngress, err = ingressClient.Create(context.Background(), backendIngress, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Failed to create backend ingress in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend ingress")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created backend ingress")

		// Update frontend deployment
		for i, container := range frontendDeploy.Spec.Template.Spec.Containers {
			if container.Name == "frontend" {
				for j, env := range frontendDeploy.Spec.Template.Spec.Containers[i].Env {
					if env.Name == "REACT_APP_API_URL" {
						frontendDeploy.Spec.Template.Spec.Containers[i].Env[j].Value = "http://" + config.Namespace + "-backend" + tld
						break
					}
				}
				break
			}
		}

		// Create frontend deployment
		frontendDeploy, err = deploymentClient.Create(context.Background(), frontendDeploy, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Failed to create frontend deployment in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create frontend deployment")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created frontend deployment")

		// Wait on the Frontend deployment to become ready
		err = waitOnDeployment(deploymentClient, frontendDeploy.Name)
		if err != nil {
			fmt.Printf("Postgresql deployment timeout - not ready in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Frontend deployment not ready")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: frontend deployment ready")

		// Create Frontend service
		service = createService("frontend", "frontend", 3000)
		service, err = serviceClient.Create(context.Background(), service, metav1.CreateOptions{})

		if err != nil {
			fmt.Printf("Failed to create frontend service in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create frontend service")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created frontend service")

		// Create frontend ingress (namespace.tld)
		frontendIngress := createIngress("frontend", 3000, config.Namespace)

		frontendIngress, err = ingressClient.Create(context.Background(), frontendIngress, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Failed to create frontend ingress in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create frontend ingress")
			return
		}
		annotateNamespaceWithStatus(namespaceClient, namespace, "Working: created frontend ingress")

		// Create backend password
		backendPassword := generatePassword()
		backendCreds := BackendUser{
			Username: "admin",
			Password: backendPassword,
		}
		userJson, _ := json.Marshal(backendCreds)
		userReader := bytes.NewReader(userJson)
		resp, err := http.Post("http://"+config.Namespace+"-backend"+tld+"/register", "application/json", userReader)
		if err != nil {
			fmt.Printf("Failed to create admin user for the backend in namespace %v. Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend admin user")
			return
		}

		if resp.StatusCode >= 300 || resp.StatusCode < 200 {
			fmt.Printf("Failed to create admin user for the backend in namespace %v\n", config.Namespace)
			fmt.Printf("Status code: %v", resp.StatusCode)
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend admin user")
			return
		}

		backendSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "backend-creds"},
			StringData: map[string]string{"username": "admin", "password": backendPassword},
		}

		// Create backend secret for admin user/pass
		backendSecret, err = clientset.CoreV1().Secrets(config.Namespace).Create(context.Background(), backendSecret, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Failed to create secret for the backend in namespace %v. Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend secret")
			return
		}

		annotateNamespaceWithStatus(namespaceClient, namespace, "Completed")

	}()

	// Respond
	w.WriteHeader(http.StatusCreated)
}

// Generate a database and backend password (8 characters in length)
func generatePassword() string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789")
	length := 8
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// Update namespace with error annotations to be read later "error" annotation
func annotateNamespaceWithStatus(namespaceClient corev1type.NamespaceInterface, namespace *corev1.Namespace, status string) {
	annotationsPatch := []byte(fmt.Sprintf(`{"metadata":{"annotations": {"status": "%s", "manager": "saas" }}}`, status))

	namespace, err := namespaceClient.Patch(context.Background(), namespace.Name, types.MergePatchType, annotationsPatch, metav1.PatchOptions{}, "")
	if err != nil {
		fmt.Printf("failed to patch namespace with status... error %v\n", err)
	}
}

// Update namespace with error annotations to be read later "error" annotation
func annotateNamespaceWithError(namespaceClient corev1type.NamespaceInterface, namespace *corev1.Namespace, errStr string) {
	annotationsPatch := []byte(fmt.Sprintf(`{"metadata":{"annotations": {"status": "Failed", "manager": "saas", "error": "%s" }}}`, errStr))

	namespace, err := namespaceClient.Patch(context.Background(), namespace.Name, types.MergePatchType, annotationsPatch, metav1.PatchOptions{}, "")
	if err != nil {
		fmt.Printf("failed to patch namespace with status... error %v\n", err)
	}
}

// Wait for a deployment to become ready
func waitOnDeployment(deploymentClient appsv1type.DeploymentInterface, deployName string) error {
	for start := time.Now(); time.Since(start) < 180*time.Second; {
		deploy, _ := deploymentClient.Get(context.Background(), deployName, metav1.GetOptions{})
		if deploy.Status.ReadyReplicas >= 1 {
			return nil
		}
	}

	return errors.New("deployment was not ready in 120 seconds")
}

// Convert namespace string to valid k8s string
func validateNamespace(namespace string) (string, error) {
	reg, err := regexp.Compile("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
	if err != nil {
		return "", errors.New("invalid namespace name")
	}

	// Validate name on the namespace
	if !reg.MatchString(namespace) || len(namespace) > 63 {
		return "", errors.New("invalid namespace name")
	}

	return namespace, nil
}
