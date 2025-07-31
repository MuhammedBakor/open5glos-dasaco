// This code is part of the 5GLOS project
// A configuration manager for consider running service inside a Kubernetes cluster or standalone at localhost
// To be built
package config

import "time"

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
