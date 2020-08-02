package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type Namespace struct {
	Namespace string `json:"namespace"`
}

func Routes(cs *kubernetes.Clientset) *chi.Mux {
	clientset = cs

	router := chi.NewRouter()
	router.Post("/saas", CreateSaaS)
	router.Get("/saas", nil)
	router.Delete("/saas", nil)
	return router
}

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

func CreateSaaS(w http.ResponseWriter, r *http.Request) {

	var config Namespace

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
		//frontendDeploy := getFrontendDeploy()

		// Create namespace
		namespace, _ := namespaceClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: config.Namespace,
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
			fmt.Printf("Postgres deployment object: %v\n", postgresDeploy)
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create postgresql deployment")
			return
		}

		// Wait on the postgresql deployment to become ready
		err = waitOnDeployment(deploymentClient, postgresDeploy.Name)
		if err != nil {
			fmt.Printf("Postgresql deployment not ready after 30 seconds in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Postgresql deployment not ready")
			return
		}

		// Create postgresql service
		serviceClient := clientset.CoreV1().Services(config.Namespace)
		service := createService("postgresql", "postgresql", 5432)
		service, err = serviceClient.Create(context.Background(), service, metav1.CreateOptions{})

		if err != nil {
			fmt.Printf("Failed to create postgresql service in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create postgresql service")
		}

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

		// Wait on the backend deployment to become ready
		err = waitOnDeployment(deploymentClient, backendDeploy.Name)
		if err != nil {
			fmt.Printf("Backend deployment not ready after 30 seconds in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Backend deployment not ready")
			return
		}

		// Create backend service
		service = createService("backend", "backend", 5432)
		service, err = serviceClient.Create(context.Background(), service, metav1.CreateOptions{})

		if err != nil {
			fmt.Printf("Failed to create backend service in namespace %v.  Error was %v\n", config.Namespace, err.Error())
			annotateNamespaceWithError(namespaceClient, namespace, "Failed to create backend service")
		}

		// Create backend ingress


		// Create backend username/password



		// Create frontend deployment

		// Create frontend service

		// Create frontend ingress

		// Done

	}()

	// Respond
	w.WriteHeader(http.StatusCreated)
}

// Update namespace with error annotations to be read later "status" annotation
func annotateNamespaceWithError(namespaceClient corev1type.NamespaceInterface, namespace *corev1.Namespace, str string) {
	annotations := namespace.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations["status"] = str
	namespace.SetAnnotations(annotations)
	_, _ = namespaceClient.Update(context.Background(), namespace, metav1.UpdateOptions{})
}

// Wait for a deployment to become ready
func waitOnDeployment(deploymentClient appsv1type.DeploymentInterface, deployName string) error {
	for start := time.Now(); time.Since(start) < 60 * time.Second; {
			deploy, _ := deploymentClient.Get(context.Background(), deployName, metav1.GetOptions{})
			if deploy.Status.ReadyReplicas >= 1 {
				return nil
			}
	}

	return errors.New("deployment was not ready in 30 seconds")
}


// Convert namespace string to valid k8s string
func validateNamespace(namespace string) (string, error) {
	reg, err := regexp.Compile("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
	if err != nil {
		return "", errors.New("invalid namespace name")
	}

	// Validate name on the namespace
	if ! reg.MatchString(namespace) || len(namespace) > 63 {
		return "", errors.New("invalid namespace name")
	}

	return namespace, nil
}
