// This code is part of the 5GLOS project
// A configuration manager for consider running service inside a Kubernetes cluster or standalone at localhost
// To be built
package config

import (
	"fmt"
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Proxy        ProxyConfig        `yaml:"proxy"`
	Kubernetes   KubernetesConfig   `yaml:"kubernetes"`
	LoadBalancer LoadBalancerConfig `yaml:"load_balancer"`
	Logging      LoggingConfig      `yaml:"logging"`
	Metrics      MetricsConfig      `yaml:"metrics"`
	Connection   ConnectionConfig   `yaml:"connection"`
}

type ProxyConfig struct {
	ListenAddr     string `yaml:"listen_addr"`
	ListenPort     int    `yaml:"listen_port"`
	ReadTimeout    string `yaml:"read_timeout"`
	WriteTimeout   string `yaml:"write_timeout"`
	BufferSize     int    `yaml:"buffer_size"`
	MaxConnections int    `yaml:"max_connections"`
}

type KubernetesConfig struct {
	Namespace      string `yaml:"namespace"`
	AMFSelector    string `yaml:"amf_selector"`
	PodTimeout     string `yaml:"pod_timeout"`
	ResyncPeriod   string `yaml:"resync_period"`
	InCluster      bool   `yaml:"in_cluster"`
	KubeconfigPath string `yaml:"kubeconfig_path"`
}

type LoadBalancerConfig struct {
	Strategy            string `yaml:"strategy"`
	HealthCheckInterval string `yaml:"health_check_interval"`
	SessionTimeout      string `yaml:"session_timeout"`
	MaxRetries          int    `yaml:"max_retries"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

type ConnectionConfig struct {
	AMF struct {
		MaxConnections    int           `yaml:"max_connections"`
		MinConnections    int           `yaml:"min_connections"`
		ConnectionTimeout time.Duration `yaml:"connection_timeout"`
		RetryInterval     time.Duration `yaml:"retry_interval"`
		LoadBalancing     string        `yaml:"load_balancing"` // "round_robin", "least_connections", "weighted"
	} `yaml:"amf"`

	GnB struct {
		SessionTimeout    time.Duration `yaml:"session_timeout"`
		MaxConcurrentUEs  int           `yaml:"max_concurrent_ues"`
		HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	} `yaml:"gnb"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configPath string) (*Config, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}
