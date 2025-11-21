package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	sevenDays = 7 * 24 * time.Hour
)

// Certificate represents a TLS certificate with its private key
type Certificate struct {
	// Cert is the X.509 certificate
	Cert *x509.Certificate

	// TLSCert is the tls.Certificate for use in TLS connections
	TLSCert tls.Certificate

	// Domains is the list of domains this certificate is valid for
	Domains []string

	// NotBefore is when the certificate becomes valid
	NotBefore time.Time

	// NotAfter is when the certificate expires
	NotAfter time.Time
}

// CertificateManager manages TLS certificates
type CertificateManager struct {
	mu sync.RWMutex

	// certificates maps domain names to certificates
	// Supports exact matches and wildcard patterns
	certificates map[string]*Certificate

	// defaultCert is used when no matching certificate is found
	defaultCert *Certificate
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager() *CertificateManager {
	return &CertificateManager{
		certificates: make(map[string]*Certificate),
	}
}

func (c *CertificateManager) LoadCertificate(certFile, keyFile string) (*Certificate, error) {

	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %w", err)
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 certificate: %w", err)
	}

	domains := make([]string, 0, len(x509Cert.DNSNames)+1)
	if x509Cert.Subject.CommonName != "" {
		domains = append(domains, x509Cert.Subject.CommonName)
	}
	domains = append(domains, x509Cert.DNSNames...)

	cert := &Certificate{
		Cert:      x509Cert,
		TLSCert:   tlsCert,
		Domains:   domains,
		NotBefore: x509Cert.NotBefore,
		NotAfter:  x509Cert.NotAfter,
	}

	return cert, nil
}

func (c *CertificateManager) AddCertificateFromFiles(certFile, keyFile string, setDefault bool) error {
	cert, err := c.LoadCertificate(certFile, keyFile)
	if err != nil {
		return err
	}

	return c.AddCertificate(cert, setDefault)
}

func (c *CertificateManager) AddCertificate(cert *Certificate, setDefault bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.validate(cert); err != nil {
		return err
	}

	for _, domain := range cert.Domains {
		c.certificates[domain] = cert
	}

	if setDefault || c.defaultCert == nil {
		c.defaultCert = cert
	}

	return nil
}

func (c *CertificateManager) validate(cert *Certificate) error {
	if cert == nil {
		return fmt.Errorf("certificate is nil")
	}

	if cert.Cert == nil {
		return fmt.Errorf("certificate x509 cert is nil")
	}

	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not valid before %s", cert.NotBefore)
	}

	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate expired at %s", cert.NotAfter)
	}

	if cert.NotAfter.Sub(now) < sevenDays {
		slog.Warn("certificate is expiring soon", "notAfter", cert.NotAfter)
	}

	return nil
}
