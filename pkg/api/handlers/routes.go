package handlers

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"net/http"
	"provisioning-api/pkg/api/k8sprobes"
	httpSwagger "github.com/swaggo/http-swagger"
)

// Bring together all routes present in any packages.
// Each package which has routes should have a Routes() function.  This function should be attached to a specific router
// API mount point here.  They can reference the root path as this will control the location of where things are mounted
func Routes() *chi.Mux {

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

	// Versions API routes
	router.Route("/v1", func(r chi.Router) {
		//TODO - Add provisioning here
	})

	// Liveness and Readiness k8s probes
	router.Route("/", func(r chi.Router) {
		r.Mount("/", k8sprobes.Routes())
	})

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:1323/swagger/doc.json"), //The url pointing to API definition"
	))


	return router
}