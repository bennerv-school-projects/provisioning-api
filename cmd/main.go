package main

import (
	"context"
	"errors"
	"github.com/bennerv/provisioning-api/pkg/api/handlers"
	"github.com/bennerv/provisioning-api/pkg/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// @title Swagger Kubernetes Provisioning API
// @version 1.0
// @description K8s Provisioning API for Order Meow

// @contact.name Ben Vesel
// @contact.email bves94 AT gmail DOT com
// @license.name MIT
// @BasePath /v1
func main() {
	if err := run(); err != nil {
		log.Println("shutting down", "error:", err)
		os.Exit(1)
	}
}

// Initializes the kubernetes go-client for an in cluster configuration using a service token
func initInClusterConfig() (*kubernetes.Clientset, error) {
	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return &kubernetes.Clientset{}, err
	}
	// creates the clientset
	return kubernetes.NewForConfig(clusterConfig)
}

// Attempt to find a kubeconfig file located at $HOME/.kube/config
// Initializes the kubernetes go-client for an out of cluster configuration
func initOutClusterConfig() (*kubernetes.Clientset, error) {
	kubeconfig := filepath.Join(
		os.Getenv("HOME"), ".kube", "config",
	)
	clusterConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	return kubernetes.NewForConfig(clusterConfig)
}

func initKubernetesClient() (*kubernetes.Clientset, error) {
	clientset, err := initInClusterConfig()
	if err != nil {
		clientset, err = initOutClusterConfig()
	}
	return clientset, err
}

func run() error {

	// initialize the logger
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	//todo Read config via a file or environment variables
	// Configuration
	cfg := config.GetConfig()

	clientSet, err := initKubernetesClient()
	if err != nil {
		panic(err.Error())
	}

	// Get all the routes out
	routeHandler := handlers.Routes(clientSet)

	// App Starting
	logger.Println("main: started")
	defer logger.Println("main: completed")

	// Create the HTTP server
	api := http.Server{
		Addr:         cfg.Web.Address,
		Handler:      routeHandler,
		ReadTimeout:  cfg.Web.ReadTimeout,
		WriteTimeout: cfg.Web.WriteTimeout,
	}

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Start the service listening for requests.
	go func() {
		logger.Printf("main : API listening on %s", api.Addr)
		serverErrors <- api.ListenAndServe()
	}()

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// =========================================================================
	// Shutdown

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		return errors.Unwrap(err)

	case <-shutdown:
		logger.Println("main : Start shutdown")

		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		// Asking listener to shutdown and load shed.
		err := api.Shutdown(ctx)
		if err != nil {
			logger.Printf("main : Graceful shutdown did not complete in %v : %v", cfg.Web.ShutdownTimeout, err)
			err = api.Close()
		}

		if err != nil {
			return errors.Unwrap(err)
		}
	}

	return nil
}
