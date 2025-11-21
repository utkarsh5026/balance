package proxy

import (
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/transport"
)

// convertTLSConfig converts conf.TLSConfig to transport.Config
func convertTLSConfig(cfg *conf.TLSConfig) (*transport.Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("TLS config is nil")
	}

	transportCfg := transport.DefaultConfig()
	if cfg.MinVersion != "" {
		minVer, err := transport.ParseTLSVersion(cfg.MinVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid min_version '%s': %w", cfg.MinVersion, err)
		}
		transportCfg.MinVersion = minVer
	}

	if cfg.MaxVersion != "" {
		maxVer, err := transport.ParseTLSVersion(cfg.MaxVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid max_version '%s': %w", cfg.MaxVersion, err)
		}
		transportCfg.MaxVersion = maxVer
	}

	if len(cfg.CipherSuites) > 0 {
		cipherSuites, err := parseCipherSuites(cfg.CipherSuites)
		if err != nil {
			return nil, fmt.Errorf("invalid cipher_suites: %w", err)
		}
		transportCfg.CipherSuites = cipherSuites
	}

	transportCfg.PreferServerCipherSuites = cfg.PreferServerCipherSuites
	transportCfg.SessionTicketsDisabled = cfg.SessionTicketsDisabled

	if cfg.ClientAuth != "" {
		clientAuth, err := parseClientAuth(cfg.ClientAuth)
		if err != nil {
			return nil, fmt.Errorf("invalid client_auth '%s': %w", cfg.ClientAuth, err)
		}
		transportCfg.ClientAuth = clientAuth
	}

	if err := transportCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid TLS configuration: %w", err)
	}

	return transportCfg, nil
}

// loadCertificateManager creates and loads certificates into a CertificateManager
func loadCertificateManager(cfg *conf.TLSConfig) (*transport.CertificateManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("TLS config is nil")
	}

	certMgr := transport.NewCertificateManager()
	if len(cfg.Certificates) > 0 {
		slog.Info("Loading multiple certificates", "count", len(cfg.Certificates))

		for i, certCfg := range cfg.Certificates {
			if certCfg.CertFile == "" || certCfg.KeyFile == "" {
				return nil, fmt.Errorf("certificate %d: cert_file and key_file are required", i)
			}

			err := certMgr.AddCertificateFromFiles(certCfg.CertFile, certCfg.KeyFile, certCfg.Default)
			if err != nil {
				return nil, fmt.Errorf("failed to load certificate %d (%s): %w", i, certCfg.CertFile, err)
			}

			slog.Info("Loaded certificate",
				"index", i,
				"cert_file", certCfg.CertFile,
				"default", certCfg.Default)
		}

		return certMgr, nil
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		slog.Info("Loading single certificate", "cert_file", cfg.CertFile)

		err := certMgr.AddCertificateFromFiles(cfg.CertFile, cfg.KeyFile, true)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate (%s): %w", cfg.CertFile, err)
		}

		return certMgr, nil
	}

	return nil, fmt.Errorf("no certificates configured: either set cert_file/key_file or use certificates list")
}

// parseCipherSuites converts cipher suite names to their uint16 values
func parseCipherSuites(names []string) ([]uint16, error) {
	cipherSuiteMap := map[string]uint16{
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":       tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":       tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,

		"TLS_RSA_WITH_AES_128_GCM_SHA256": tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_RSA_WITH_AES_256_GCM_SHA384": tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	}

	suites := make([]uint16, 0, len(names))
	for _, name := range names {
		suite, ok := cipherSuiteMap[name]
		if !ok {
			return nil, fmt.Errorf("unknown cipher suite: %s", name)
		}
		suites = append(suites, suite)
	}

	if len(suites) == 0 {
		return nil, fmt.Errorf("no valid cipher suites specified")
	}

	return suites, nil
}

// parseClientAuth converts client auth string to tls.ClientAuthType
func parseClientAuth(auth string) (tls.ClientAuthType, error) {
	switch auth {
	case "", "none":
		return tls.NoClientCert, nil
	case "request":
		return tls.RequestClientCert, nil
	case "require":
		return tls.RequireAnyClientCert, nil
	case "verify":
		return tls.VerifyClientCertIfGiven, nil
	case "require-and-verify":
		return tls.RequireAndVerifyClientCert, nil
	default:
		return 0, fmt.Errorf("unknown client auth type: %s", auth)
	}
}

// createTerminator creates a new TLS terminator from config
func createTerminator(cfg *conf.TLSConfig) (*transport.Terminator, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, fmt.Errorf("TLS is not enabled")
	}

	transportCfg, err := convertTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert TLS config: %w", err)
	}

	certMgr, err := loadCertificateManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates: %w", err)
	}

	terminator, err := transport.NewTerminator(transportCfg, certMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS terminator: %w", err)
	}

	return terminator, nil
}
