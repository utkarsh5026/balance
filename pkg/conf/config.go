package conf

import (
	"fmt"
	"os"
	"slices"
	"time"

	"go.yaml.in/yaml/v2"
	"golang.org/x/sync/errgroup"
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

	// HTTP configuration (for HTTP mode)
	HTTP *HTTPConfig `yaml:"http,omitempty"`

	// Health check configuration (optional)
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`

	// Timeouts configuration
	Timeouts TimeoutConfig `yaml:"timeouts"`

	// Metrics configuration
	Metrics MetricsConfig `yaml:"metrics"`

	// Security configuration
	Security *SecurityConfig `yaml:"security,omitempty"`
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

	var g errgroup.Group

	g.Go(c.validateTLS)
	g.Go(c.validateSecurity)

	return g.Wait()
}

func (c *Config) validateTLS() error {
	if c.TLS == nil || !c.TLS.Enabled {
		return nil
	}

	if len(c.TLS.Certificates) == 0 && (c.TLS.CertFile == "" || c.TLS.KeyFile == "") {
		return fmt.Errorf("TLS certificates or cert_file/key_file is required when TLS is enabled")
	}

	if c.TLS.KeyFile == "" {
		return fmt.Errorf("TLS key_file is required when TLS is enabled")
	}

	validVersions := map[string]bool{"1.0": true, "1.1": true, "1.2": true, "1.3": true}

	if c.TLS.MinVersion != "" {
		if !validVersions[c.TLS.MinVersion] {
			return fmt.Errorf("invalid TLS min_version: %s (must be 1.0, 1.1, 1.2, or 1.3)", c.TLS.MinVersion)
		}
	}

	if c.TLS.MaxVersion != "" {
		if !validVersions[c.TLS.MaxVersion] {
			return fmt.Errorf("invalid TLS max_version: %s (must be 1.0, 1.1, 1.2, or 1.3)", c.TLS.MaxVersion)
		}
	}

	validClientAuth := map[string]bool{
		"none": true, "request": true, "require": true,
		"verify": true, "require-and-verify": true,
	}

	if c.TLS.ClientAuth != "" {
		if !validClientAuth[c.TLS.ClientAuth] {
			return fmt.Errorf("invalid TLS client_auth: %s", c.TLS.ClientAuth)
		}
	}

	return nil
}

func (c *Config) validateSecurity() error {
	if c.Security == nil {
		return nil
	}

	validBuckets := []string{"token-bucket", "sliding-window"}
	if c.Security.RateLimit != nil && c.Security.RateLimit.Enabled {
		if !slices.Contains(validBuckets, c.Security.RateLimit.Type) {
			return fmt.Errorf("invalid rate limit type: %s (must be 'token-bucket' or 'sliding-window')", c.Security.RateLimit.Type)
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
