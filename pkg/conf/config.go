package conf

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v2"
)

type Config struct {
	// Mode can be "tcp" or "http"
	Mode string `yaml:"mode"`

	// Listen address (e.g., ":8080" or "0.0.0.0:8080")
	Listen string `yaml:"listen"`

	// Nodes configuration
	Nodes []Node `yaml:"nodes"`

	// LoadBalancer configuration
	LoadBalancer LoadBalancerConfig `yaml:"load_balancer"`

	// TLS configuration (optional)
	TLS *TLSConfig `yaml:"tls,omitempty"`

	// Health check configuration (optional)
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`

	// Timeouts configuration
	Timeouts TimeoutConfig `yaml:"timeouts"`

	// Metrics configuration
	Metrics MetricsConfig `yaml:"metrics"`
}

func Load(path string) (*Config, error) {
	data, error := os.ReadFile(path)
	if error != nil {
		return nil, error
	}

	var config Config
	if error := yaml.Unmarshal(data, &config); error != nil {
		return nil, error
	}

	return &config, nil
}

func (c *Config) Validate() error {

	if c.Mode != "tcp" && c.Mode != "http" {
		return fmt.Errorf("invalid mode: %s (must be 'tcp' or 'http')", c.Mode)
	}

	if len(c.Nodes) == 0 {
		return fmt.Errorf("at least one node must be configured")
	}

	for i, node := range c.Nodes {
		if node.Address == "" {
			return fmt.Errorf("node %d has empty address", i)
		}

		if node.Weight < 0 {
			return fmt.Errorf("node %d has negative weight", i)
		}
	}

	validAlgorithms := map[string]bool{
		"round-robin":          true,
		"least-connections":    true,
		"consistent-hash":      true,
		"weighted-round-robin": true,
	}
	if !validAlgorithms[c.LoadBalancer.Algorithm] {
		return fmt.Errorf("invalid load balancer algorithm: %s", c.LoadBalancer.Algorithm)
	}

	if c.TLS != nil && c.TLS.Enabled {
		if c.TLS.CertFile == "" {
			return fmt.Errorf("TLS cert_file is required when TLS is enabled")
		}
		if c.TLS.KeyFile == "" {
			return fmt.Errorf("TLS key_file is required when TLS is enabled")
		}
	}

	return nil
}
