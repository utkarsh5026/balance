package conf

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	// Enabled enables rate limiting
	Enabled bool `yaml:"enabled"`

	// Type: "token-bucket" or "sliding-window"
	Type string `yaml:"type"`

	// RequestsPerSecond for token bucket rate limiting
	RequestsPerSecond float64 `yaml:"requests_per_second,omitempty"`

	// BurstSize for token bucket (max tokens)
	BurstSize int64 `yaml:"burst_size,omitempty"`

	// WindowSize for sliding window rate limiting (e.g., "1m", "1h")
	WindowSize string `yaml:"window_size,omitempty"`

	// MaxRequests for sliding window rate limiting
	MaxRequests int64 `yaml:"max_requests,omitempty"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	// RateLimit configuration
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
}
