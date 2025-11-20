package conf

import (
	"fmt"
	"os"
	"time"

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

func Load(path string, fillWithDefault bool) (*Config, error) {
	data, error := os.ReadFile(path)
	if error != nil {
		return nil, error
	}

	var config Config
	if error := yaml.Unmarshal(data, &config); error != nil {
		return nil, error
	}

	err := config.Validate()
	if err != nil && fillWithDefault {
		config.setDefaults()
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

func (c *Config) setDefaults() {
	if c.Mode == "" {
		c.Mode = "tcp"
	}

	if c.LoadBalancer.Algorithm == "" {
		c.LoadBalancer.Algorithm = "round-robin"
	}

	for i := range c.Nodes {
		if c.Nodes[i].Weight == 0 {
			c.Nodes[i].Weight = 1
		}
	}

	if c.Timeouts.Connect == 0 {
		c.Timeouts.Connect = 5 * time.Second
	}

	if c.Timeouts.Read == 0 {
		c.Timeouts.Read = 30 * time.Second
	}

	if c.Timeouts.Write == 0 {
		c.Timeouts.Write = 30 * time.Second
	}

	if c.Timeouts.Idle == 0 {
		c.Timeouts.Idle = 90 * time.Second
	}

	if c.HealthCheck != nil && c.HealthCheck.Enabled {
		if c.HealthCheck.Interval == 0 {
			c.HealthCheck.Interval = 10 * time.Second
		}

		if c.HealthCheck.Timeout == 0 {
			c.HealthCheck.Timeout = 2 * time.Second
		}

		if c.HealthCheck.UnhealthyThreshold == 0 {
			c.HealthCheck.UnhealthyThreshold = 3
		}

		if c.HealthCheck.HealthyThreshold == 0 {
			c.HealthCheck.HealthyThreshold = 2
		}
	}
}
