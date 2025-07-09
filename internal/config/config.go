/*
Holds Yaml/env configuration loader for the 5GLOS project.
*/
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Proxy        ProxyConfig        `yaml:"proxy"`
	Kubernetes   KubernetesConfig   `yaml:"kubernetes"`
	LoadBalancer LoadBalancerConfig `yaml:"load_balancer"`
	Logging      LoggingConfig      `yaml:"logging"`
	Metrics      MetricsConfig      `yaml:"metrics"`
}

// ProxyConfig contains SCTP proxy settings
type ProxyConfig struct {
	ListenAddr     string        `yaml:"listen_addr"`
	ListenPort     int           `yaml:"listen_port"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	BufferSize     int           `yaml:"buffer_size"`
	MaxConnections int           `yaml:"max_connections"`
}

// KubernetesConfig contains Kubernetes-related settings
type KubernetesConfig struct {
	Namespace      string        `yaml:"namespace"`
	AMFSelector    string        `yaml:"amf_selector"`
	PodTimeout     time.Duration `yaml:"pod_timeout"`
	ResyncPeriod   time.Duration `yaml:"resync_period"`
	InCluster      bool          `yaml:"in_cluster"`
	KubeconfigPath string        `yaml:"kubeconfig_path"`
}

// LoadBalancerConfig contains load balancing settings
type LoadBalancerConfig struct {
	Strategy            string        `yaml:"strategy"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	SessionTimeout      time.Duration `yaml:"session_timeout"`
	MaxRetries          int           `yaml:"max_retries"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// MetricsConfig contains metrics server settings
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// Load reads and parses the configuration file
func Load(configPath string) (*Config, error) {
	// Set defaults
	cfg := &Config{
		Proxy: ProxyConfig{
			ListenAddr:     "0.0.0.0",
			ListenPort:     38412,
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			BufferSize:     4096,
			MaxConnections: 1000,
		},
		Kubernetes: KubernetesConfig{
			Namespace:      "free5gc",
			AMFSelector:    "app=amf",
			PodTimeout:     5 * time.Minute,
			ResyncPeriod:   30 * time.Second,
			InCluster:      false,
			KubeconfigPath: "",
		},
		LoadBalancer: LoadBalancerConfig{
			Strategy:            "round-robin",
			HealthCheckInterval: 10 * time.Second,
			SessionTimeout:      30 * time.Minute,
			MaxRetries:          3,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
	}

	// Read configuration file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// validate checks the configuration for required fields and valid values
func (c *Config) validate() error {
	if c.Proxy.ListenPort <= 0 || c.Proxy.ListenPort > 65535 {
		return fmt.Errorf("proxy.listen_port must be between 1 and 65535")
	}

	if c.Kubernetes.Namespace == "" {
		return fmt.Errorf("kubernetes.namespace is required")
	}

	if c.Kubernetes.AMFSelector == "" {
		return fmt.Errorf("kubernetes.amf_selector is required")
	}

	validStrategies := map[string]bool{
		"round-robin":       true,
		"least-connections": true,
		"weighted":          true,
	}
	if !validStrategies[c.LoadBalancer.Strategy] {
		return fmt.Errorf("load_balancer.strategy must be one of: round-robin, least-connections, weighted")
	}

	if c.Metrics.Port <= 0 || c.Metrics.Port > 65535 {
		return fmt.Errorf("metrics.port must be between 1 and 65535")
	}

	return nil
}
