package provisioner

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"strings"
)

const tld string = ".bennerv.com"

// Generic postgres deployment object
func getPostgresDeploy() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "postgresql",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "postgresql",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "postgresql",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgresql",
							Image: "postgres:12.3-alpine",
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_DB",
									Value: "postgresdb",
								},
								{
									Name:  "POSTGRES_USER",
									Value: "postgresuser",
								},
								{
									Name:  "POSTGRES_PASSWORD",
									Value: "",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"memory": resource.MustParse("50Mi"),
									"cpu":    resource.MustParse("50m"),
								},
								Limits: corev1.ResourceList{
									"memory": resource.MustParse("250Mi"),
									"cpu":    resource.MustParse("1000m"),
								},
							},
						},
					},
				},
			},
			Strategy:             appsv1.DeploymentStrategy{},
			MinReadySeconds:      0,
			RevisionHistoryLimit: pointer.Int32Ptr(2),
		},
	}
}

// Backend deployment object
func getBackendDeploy() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "backend",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "backend",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "backend",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "backend",
							Image: "bennerv/order-meow-api:0.1.0",
							Env: []corev1.EnvVar{
								{
									Name:  "SPRING_DATASOURCE_URL",
									Value: "jdbc:postgresql://postgresql:5432/postgresdb",
								},
								{
									Name:  "SPRING_DATASOURCE_USERNAME",
									Value: "postgresuser",
								},
								{
									Name:  "SPRING_DATASOURCE_PASSWORD",
									Value: "",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"memory": resource.MustParse("250Mi"),
									"cpu":    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									"memory": resource.MustParse("2Gi"),
									"cpu":    resource.MustParse("2000m"),
								},
							},
						},
					},
				},
			},
			Strategy:             appsv1.DeploymentStrategy{},
			MinReadySeconds:      0,
			RevisionHistoryLimit: pointer.Int32Ptr(2),
		},
	}
}

// Frontend deployment object
func getFrontendDeploy() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "frontend",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "frontend",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "frontend",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "frontend",
							Image: "bennerv/order-meow-ui:0.1.2",
							Env: []corev1.EnvVar{
								{
									Name:  "REACT_APP_API_URL",
									Value: "",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"memory": resource.MustParse("50Mi"),
									"cpu":    resource.MustParse("50m"),
								},
								Limits: corev1.ResourceList{
									"memory": resource.MustParse("1Gi"),
									"cpu":    resource.MustParse("1000m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: int32(3000),
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      0,
								PeriodSeconds:       0,
								SuccessThreshold:    0,
								FailureThreshold:    0,
							},
						},
					},
				},
			},
			Strategy:             appsv1.DeploymentStrategy{},
			MinReadySeconds:      0,
			RevisionHistoryLimit: pointer.Int32Ptr(2),
		},
	}
}

// Create a pointer to a service with the following params
func createService(name string, matcher string, port int) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: name,
					Port: int32(port),
				},
			},
			Selector: map[string]string{
				"app": matcher,
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// Create a pointer to an ingress with the following params
func createIngress(service string, port int, namespace string) *netv1beta1.Ingress {
	var host string
	if strings.ToLower(service) == "backend" {
		host = namespace + "-backend" + tld
	} else {
		host = namespace + tld
	}

	return &netv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: service,
		},
		Spec: netv1beta1.IngressSpec{
			Backend: &netv1beta1.IngressBackend{
				ServiceName: service,
				ServicePort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: int32(port),
				},
			},
			Rules: []netv1beta1.IngressRule{
				{
					Host: host,
				},
			},
		},
	}
}
