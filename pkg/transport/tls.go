package transport

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"slices"
)

type TLSVersion uint16

const (
	// TLS versions
	VersionTLS10 TLSVersion = tls.VersionTLS10
	VersionTLS11 TLSVersion = tls.VersionTLS11
	VersionTLS12 TLSVersion = tls.VersionTLS12
	VersionTLS13 TLSVersion = tls.VersionTLS13
)

func (v TLSVersion) String() string {
	switch v {
	case VersionTLS10:
		return "TLS 1.0"
	case VersionTLS11:
		return "TLS 1.1"
	case VersionTLS12:
		return "TLS 1.2"
	case VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (%d)", v)
	}
}

var (
	ErrUnsupportedTLSVersion = fmt.Errorf("unsupported TLS version")
)

// Config represents TLS configuration parameters.
type Config struct {
	// MinVersion contains the minimum TLS version that is acceptable.
	// If zero, TLS 1.0 is currently taken as the minimum.
	MinVersion TLSVersion

	// MaxVersion contains the maximum TLS version that is acceptable.
	// If zero, the maximum version supported by the package is used,
	// which is currently TLS 1.3.
	MaxVersion TLSVersion

	// CipherSuites is a list of enabled cipher suites for TLS versions up to TLS 1.2.
	// The order of the list is ignored unless PreferServerCipherSuites is true.
	// If nil, a safe default list is used.
	// Note that TLS 1.3 cipher suites are not configurable.
	CipherSuites []uint16

	// PreferServerCipherSuites controls whether the server selects the
	// client's most preferred cipher suite, or the server's most preferred
	// cipher suite. If true then the server's preference, as expressed in
	// the order of elements in CipherSuites, is used.
	PreferServerCipherSuites bool

	// SessionTicketsDisabled may be set to true to disable session ticket
	// (resumption) support.
	SessionTicketsDisabled bool

	// SessionTicketKey is used by TLS servers to provide session resumption.
	// If zero, it will be filled with random data before the first server
	// handshake.
	//
	// If multiple servers are terminating connections for the same host
	// they should all have the same SessionTicketKey.
	SessionTicketKey [32]byte

	// ClientAuth determines the server's policy for
	// TLS Client Authentication. The default is NoClientCert.
	ClientAuth tls.ClientAuthType

	// NextProtos is a list of supported application level protocols, in
	// order of preference.
	// Example: []string{"h2", "http/1.1"}
	NextProtos []string

	// InsecureSkipVerify controls whether a client verifies the server's
	// certificate chain and host name.
	// If InsecureSkipVerify is true, crypto/tls accepts any certificate
	// presented by the server and any host name in that certificate.
	// In this mode, TLS is susceptible to machine-in-the-middle attacks.
	// This should be used only for testing.
	InsecureSkipVerify bool

	// Renegotiation controls what types of renegotiation are supported.
	// The default, never renegotiate, is correct for the vast majority of
	// applications.
	Renegotiation tls.RenegotiationSupport
}

// DefaultConfig returns a secure default TLS configuration
func DefaultConfig() *Config {
	return &Config{
		MinVersion:               VersionTLS12,
		MaxVersion:               VersionTLS13,
		CipherSuites:             SecureCipherSuites(),
		PreferServerCipherSuites: true,
		SessionTicketsDisabled:   false,
		ClientAuth:               tls.NoClientCert,
		NextProtos:               []string{"h2", "http/1.1"},
		InsecureSkipVerify:       false,
		Renegotiation:            tls.RenegotiateNever,
	}
}

// SecureCipherSuites returns a list of secure cipher suites
// These are recommended cipher suites as of 2024
func SecureCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 cipher suites (automatically enabled when using TLS 1.3)
		// TLS 1.3 doesn't use the CipherSuites field

		// TLS 1.2 cipher suites (ECDHE with AES-GCM or ChaCha20-Poly1305)
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	}
}

// ParseTLSVersion parses a string TLS version to TLSVersion
func ParseTLSVersion(version string) (TLSVersion, error) {
	switch version {
	case "1.0":
		return VersionTLS10, nil
	case "1.1":
		return VersionTLS11, nil
	case "1.2":
		return VersionTLS12, nil
	case "1.3":
		return VersionTLS13, nil
	default:
		return 0, ErrUnsupportedTLSVersion
	}
}

// ToStdConfig converts our TLS config to crypto/tls.Config
func (c *Config) ToStdConfig() *tls.Config {
	cfg := &tls.Config{
		MinVersion:               uint16(c.MinVersion),
		MaxVersion:               uint16(c.MaxVersion),
		CipherSuites:             c.CipherSuites,
		PreferServerCipherSuites: c.PreferServerCipherSuites,
		SessionTicketsDisabled:   c.SessionTicketsDisabled,
		ClientAuth:               c.ClientAuth,
		NextProtos:               c.NextProtos,
		InsecureSkipVerify:       c.InsecureSkipVerify,
		Renegotiation:            c.Renegotiation,
	}

	return cfg
}

// Validate validates the TLS configuration
func (c *Config) Validate() error {
	if c.MinVersion < VersionTLS10 || c.MinVersion > VersionTLS13 {
		return fmt.Errorf("invalid minimum TLS version: %d", c.MinVersion)
	}

	if c.MaxVersion != 0 && (c.MaxVersion < VersionTLS10 || c.MaxVersion > VersionTLS13) {
		return fmt.Errorf("invalid maximum TLS version: %d", c.MaxVersion)
	}

	if c.MaxVersion != 0 && c.MinVersion > c.MaxVersion {
		return fmt.Errorf("minimum TLS version (%d) cannot be greater than maximum version (%d)", c.MinVersion, c.MaxVersion)
	}

	if c.MinVersion < VersionTLS12 {
		slog.Warn("TLS versions below 1.2 are considered insecure and not recommended")
	}

	return nil
}

func (c *Config) Clone() *Config {
	clone := &Config{
		MinVersion:               c.MinVersion,
		MaxVersion:               c.MaxVersion,
		PreferServerCipherSuites: c.PreferServerCipherSuites,
		SessionTicketsDisabled:   c.SessionTicketsDisabled,
		SessionTicketKey:         c.SessionTicketKey,
		ClientAuth:               c.ClientAuth,
		InsecureSkipVerify:       c.InsecureSkipVerify,
		Renegotiation:            c.Renegotiation,
	}

	if c.CipherSuites != nil {
		clone.CipherSuites = slices.Clone(c.CipherSuites)
	}

	if c.NextProtos != nil {
		clone.NextProtos = slices.Clone(c.NextProtos)
	}

	return clone
}
