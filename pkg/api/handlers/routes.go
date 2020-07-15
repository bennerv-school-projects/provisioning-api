package handlers

import (
	"github.com/bennerv/provisioning-api/pkg/api/k8sprobes"
	"github.com/bennerv/provisioning-api/pkg/api/provisioner"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"k8s.io/client-go/kubernetes"
	"net/http"
)

// Bring together all routes present in any packages.
// Each package which has routes should have a Routes() function.  This function should be attached to a specific router
// API mount point here.  They can reference the root path as this will control the location of where things are mounted
func Routes(clientset *kubernetes.Clientset) *chi.Mux {

	router := chi.NewRouter()
	router.Use(

		middleware.Logger,

		// Redirect slashes to the correct endpoint
		middleware.RedirectSlashes,

		// Allow recovery from failed middleware
		middleware.Recoverer,

		// Set request header content type Json
		render.SetContentType(render.ContentTypeJSON),

		// Set response header to always be ContentType: application/json
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				next.ServeHTTP(w, r)
			})
		},
	)

	// Versioned API routes for provisioner
	router.Route("/v1", func(r chi.Router) {
		r.Mount("/", provisioner.Routes(clientset))
	})

	// Liveness and Readiness k8s probes
	router.Route("/", func(r chi.Router) {
		r.Mount("/", k8sprobes.Routes())
	})

	//router.Get("/swagger/*", httpSwagger.Handler(
	//	httpSwagger.URL(":8080/swagger/doc.json"), //The url pointing to API definition"
	//))

	return router
}
