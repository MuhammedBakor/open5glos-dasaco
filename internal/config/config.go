package config

import (
	"encoding/json"
	"os"
)

// Config holds the application configuration settings.
type Config struct {
	MinikubeIP string `json:"minikube_ip"`
	Namespace  string `json:"namespace"`
}

// LoadConfig loads the configuration from a JSON file.
func LoadConfig(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}