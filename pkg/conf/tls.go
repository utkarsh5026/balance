package conf

// TLSConfig represents TLS/SSL configuration
type TLSConfig struct {
	// Enabled enables TLS termination
	Enabled bool `yaml:"enabled"`

	// CertFile path to certificate file
	CertFile string `yaml:"cert_file"`

	// KeyFile path to private key file
	KeyFile string `yaml:"key_file"`

	// MinVersion minimum TLS version (e.g., "1.0", "1.1", "1.2", "1.3")
	MinVersion string `yaml:"min_version,omitempty"`

	// MaxVersion maximum TLS version (e.g., "1.3")
	MaxVersion string `yaml:"max_version,omitempty"`

	// Certificates is a list of certificate configurations for multi-domain support
	Certificates []CertificateConfig `yaml:"certificates,omitempty"`

	// ClientAuth determines the server's policy for client authentication
	// Options: "none", "request", "require", "verify", "require-and-verify"
	ClientAuth string `yaml:"client_auth,omitempty"`

	// PreferServerCipherSuites controls whether server cipher suite preferences are used
	PreferServerCipherSuites bool `yaml:"prefer_server_cipher_suites"`

	// SessionTicketsDisabled disables session ticket (resumption) support
	SessionTicketsDisabled bool `yaml:"session_tickets_disabled"`

	// CipherSuites is a list of enabled cipher suites (empty = use secure defaults)
	CipherSuites []string `yaml:"cipher_suites,omitempty"`
}

// CertificateConfig represents a single certificate configuration
type CertificateConfig struct {
	// CertFile path to certificate file
	CertFile string `yaml:"cert_file"`

	// KeyFile path to private key file
	KeyFile string `yaml:"key_file"`

	// MinVersion minimum TLS version (e.g., "1.2", "1.3")
	MinVersion string `yaml:"min_version"`
	// Domains is a list of domains this certificate is valid for (optional, auto-detected from cert)
	Domains []string `yaml:"domains,omitempty"`

	// Default indicates this is the default certificate
	Default bool `yaml:"default,omitempty"`
}
