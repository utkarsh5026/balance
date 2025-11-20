package conf

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
