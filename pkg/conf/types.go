package conf

import "time"

// MetricsConfig represents metrics configuration
type MetricsConfig struct {
	// Enabled enables Prometheus metrics
	Enabled bool `yaml:"enabled"`

	// Listen address for metrics endpoint (e.g., ":9090")
	Listen string `yaml:"listen"`

	// Path for metrics endpoint (default: "/metrics")
	Path string `yaml:"path"`
}

// TimeoutConfig represents timeout settings
type TimeoutConfig struct {
	// Connect timeout for connecting to backends
	Connect time.Duration `yaml:"connect"`

	// Read timeout for reading from connections
	Read time.Duration `yaml:"read"`

	// Write timeout for writing to connections
	Write time.Duration `yaml:"write"`

	// Idle timeout for idle connections
	Idle time.Duration `yaml:"idle"`
}

// HealthCheckConfig represents health check settings
type HealthCheckConfig struct {
	// Enabled enables health checking
	Enabled bool `yaml:"enabled"`

	// Interval between health checks
	Interval time.Duration `yaml:"interval"`

	// Timeout for health check requests
	Timeout time.Duration `yaml:"timeout"`

	// UnhealthyThreshold number of failures before marking unhealthy
	UnhealthyThreshold int `yaml:"unhealthy_threshold"`

	// HealthyThreshold number of successes before marking healthy
	HealthyThreshold int `yaml:"healthy_threshold"`

	// Type of health check: "tcp", "http", or "https" (auto-detected if not specified)
	Type string `yaml:"type,omitempty"`

	// Path for HTTP health checks (e.g., "/health")
	Path string `yaml:"path,omitempty"`

	// EnablePassiveChecks enables passive health monitoring based on real traffic
	EnablePassiveChecks bool `yaml:"enable_passive_checks"`

	// ErrorRateThreshold is the error rate (0.0-1.0) that triggers unhealthy state
	ErrorRateThreshold float64 `yaml:"error_rate_threshold"`

	// ConsecutiveFailures is the number of consecutive failures before marking unhealthy
	ConsecutiveFailures int `yaml:"consecutive_failures"`

	// PassiveCheckWindow is the time window for passive health check tracking
	PassiveCheckWindow time.Duration `yaml:"passive_check_window"`
}

// LoadBalancerConfig represents load balancer settings
type LoadBalancerConfig struct {
	// Algorithm: "round-robin", "least-connections", "consistent-hash", "weighted-round-robin"
	Algorithm string `yaml:"algorithm"`

	// HashKey for consistent hashing (e.g., "source-ip", "header:X-User-ID")
	HashKey string `yaml:"hash_key,omitempty"`
}

// Node represents a backend server configuration
type Node struct {
	// Name of the backend
	Name string `yaml:"name"`

	// Address of the backend (host:port)
	Address string `yaml:"address"`

	// Weight for weighted load balancing (default: 1)
	Weight int `yaml:"weight"`

	// MaxConnections limits concurrent connections to this backend (0 = unlimited)
	MaxConnections int `yaml:"max_connections"`
}

type Route struct {
	// Name of the route
	Name string `yaml:"name"`

	// Host pattern for host-based routing (e.g., "api.example.com")
	Host string `yaml:"host,omitempty"`

	// PathPrefix for path-based routing (e.g., "/api/")
	PathPrefix string `yaml:"path_prefix,omitempty"`

	// Headers for header-based routing (e.g., {"X-API-Key": "secret"})
	Headers map[string]string `yaml:"headers,omitempty"`

	// Backends for this route (backend names)
	Backends []string `yaml:"backends"`

	// Priority for route matching (higher = higher priority)
	Priority int `yaml:"priority"`
}

// HTTPConfig represents HTTP-specific configuration
type HTTPConfig struct {
	// Routes for HTTP routing (optional, if empty uses default backend pool)
	Routes []Route `yaml:"routes,omitempty"`

	// EnableWebSocket enables WebSocket proxying
	EnableWebSocket bool `yaml:"enable_websocket"`

	// EnableHTTP2 enables HTTP/2 support
	EnableHTTP2 bool `yaml:"enable_http2"`

	// MaxIdleConnsPerHost limits idle connections per backend
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host"`

	// IdleConnTimeout is the idle connection timeout
	IdleConnTimeout time.Duration `yaml:"idle_conn_timeout"`
}
