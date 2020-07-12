package config

import (
	"os"
	"time"
)

type web struct {
	Address         string        `config:"default::8080"`
	ReadTimeout     time.Duration `config:"default:5s"`
	WriteTimeout    time.Duration `config:"default:5s"`
	ShutdownTimeout time.Duration `config:"default:5s"`
}

// Stores application configuration
type Config struct {
	Web web
}

// Read in configuration from environment variables
func GetConfig() *Config {

	//TODO - Use a better configuration parsing mechanism (github.com/spf13/viper)
	config := newConfig()

	// Address
	val, ok := os.LookupEnv("PORT")
	if ok {
		config.Web.Address = val
	}

	// TODO - Add more configs as needed

	return config
}

// Create a new config with defaults
func newConfig() *Config {
	return &Config{
		Web: web{
			Address:         ":8080",
			ReadTimeout:     time.Second * 5,
			WriteTimeout:    time.Second * 5,
			ShutdownTimeout: time.Second * 5,
		},
	}
}
