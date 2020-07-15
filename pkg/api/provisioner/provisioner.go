package provisioner

import (
	"github.com/go-chi/chi"
	"k8s.io/client-go/kubernetes"
	"net/http"
)

var clientset *kubernetes.Clientset

func Routes(cs *kubernetes.Clientset) *chi.Mux {
	clientset = cs

	router := chi.NewRouter()
	router.Post("/saas", CreateSaaS)
	router.Get("/saas", nil)
	router.Delete("/saas", nil)
	return router
}

func createNamespace(w http.ResponseWriter, r *http.Request) {

}


func CreateSaaS(w http.ResponseWriter, r *http.Request) {

}
