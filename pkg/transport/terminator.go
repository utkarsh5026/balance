package transport

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
)

type Terminator struct {
	config    *Config
	certMgr   *CertificateManager
	listener  net.Listener
	tlsConfig *tls.Config
}

func NewTerminator(config *Config, cm *CertificateManager) (*Terminator, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid TLS config: %w", err)
	}

	if cm == nil {
		return nil, fmt.Errorf("certificate manager is required")
	}

	t := &Terminator{
		config:  config,
		certMgr: cm,
	}

	t.tlsConfig = t.buildTLSConfig()
	return t, nil
}

func (t *Terminator) buildTLSConfig() *tls.Config {
	tlsConfig := t.config.ToStdConfig()
	tlsConfig.GetCertificate = t.certMgr.GetCertificate

	if !t.config.SessionTicketsDisabled {
		tlsConfig.SessionTicketsDisabled = false
		if t.config.SessionTicketKey != [32]byte{} {
			tlsConfig.SetSessionTicketKeys([][32]byte{t.config.SessionTicketKey})
		}
	}

	return tlsConfig
}

// Listen creates a TLS listener on the specified address
func (t *Terminator) Listen(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	t.listener = tls.NewListener(listener, t.tlsConfig)

	slog.Info("TLS listener started", "address", address)
	slog.Info("TLS configuration",
		"min_version", t.config.MinVersion,
		"cipher_suites", len(t.config.CipherSuites))

	return nil
}

// Accept accepts a new TLS connection
func (t *Terminator) Accept() (net.Conn, error) {
	if t.listener == nil {
		return nil, fmt.Errorf("listener not initialized")
	}

	conn, err := t.listener.Accept()
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Close closes the inner lisener
func (t *Terminator) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// Addr returns the listener's network address
func (t *Terminator) Addr() net.Addr {
	if t.listener != nil {
		return t.listener.Addr()
	}
	return nil
}
