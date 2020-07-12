package k8sprobes

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"net/http"
)

func Routes() *chi.Mux {
	router := chi.NewRouter()
	router.Get("/health", GetLiveness)
	router.Get("/ready", GetReadiness)
	return router
}

func GetLiveness(w http.ResponseWriter, r *http.Request) {
	response := make(map[string]string)
	response["status"] = "OK"
	w.WriteHeader(http.StatusOK)
	render.JSON(w, r, response)
}

func GetReadiness(w http.ResponseWriter, r *http.Request) {
	response := make(map[string]string)
	response["status"] = "OK"
	w.WriteHeader(http.StatusOK)
	render.JSON(w, r, response)
}
