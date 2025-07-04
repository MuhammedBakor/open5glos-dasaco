/*
Holds Yaml/env configuration loader for the 5GLOS project.
*/
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/hasukiHT/5glos/utils"
)

// Config represents the application configuration
type Config struct {
	ListenPort           int                `yaml:"listen_port"`
	Namespace            string             `yaml:"namespace"`
	AMFLabel             string             `yaml:"amf_label"`
	LoadBalancerStrategy string             `yaml:"load_balancer_strategy"`
	Logging              utils.LoggerConfig `yaml:"logging"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configPath string) (*Config, error) {
	// Set default config path if not provided
	if configPath == "" {
		configPath = "config.yaml"
	}

	// Read the YAML file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML into Config struct
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// Validate required fields
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// validate checks if required configuration fields are set
func (c *Config) validate() error {
	if c.ListenPort <= 0 {
		return fmt.Errorf("listen_port must be a positive integer")
	}
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if c.AMFLabel == "" {
		return fmt.Errorf("amf_label is required")
	}
	if c.LoadBalancerStrategy == "" {
		return fmt.Errorf("load_balancer_strategy is required")
	}
	return nil
}

// LoadConfigFromDefault loads configuration from the default config.yaml file
func LoadConfigFromDefault() (*Config, error) {
	return LoadConfig("")
}
