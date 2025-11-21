package transport

import (
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateSelfSignedCertificate(t *testing.T) {
	t.Run("single domain", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cert == nil {
			t.Fatal("expected certificate, got nil")
		}

		if cert.Cert == nil {
			t.Fatal("expected x509 certificate, got nil")
		}

		if cert.Cert.Subject.CommonName != "example.com" {
			t.Errorf("expected CommonName 'example.com', got '%s'", cert.Cert.Subject.CommonName)
		}

		if len(cert.Cert.DNSNames) != 1 {
			t.Errorf("expected 1 DNS name, got %d", len(cert.Cert.DNSNames))
		}

		if cert.Cert.DNSNames[0] != "example.com" {
			t.Errorf("expected DNS name 'example.com', got '%s'", cert.Cert.DNSNames[0])
		}

		if len(cert.Domains) != 1 || cert.Domains[0] != "example.com" {
			t.Errorf("expected Domains to contain 'example.com', got %v", cert.Domains)
		}
	})

	t.Run("multiple domains", func(t *testing.T) {
		domains := []string{"example.com", "www.example.com", "api.example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cert.Cert.Subject.CommonName != "example.com" {
			t.Errorf("expected CommonName 'example.com', got '%s'", cert.Cert.Subject.CommonName)
		}

		if len(cert.Cert.DNSNames) != 3 {
			t.Errorf("expected 3 DNS names, got %d", len(cert.Cert.DNSNames))
		}

		for i, expected := range domains {
			if cert.Cert.DNSNames[i] != expected {
				t.Errorf("expected DNS name '%s' at index %d, got '%s'", expected, i, cert.Cert.DNSNames[i])
			}
		}

		if len(cert.Domains) != 3 {
			t.Errorf("expected 3 domains, got %d", len(cert.Domains))
		}
	})

	t.Run("wildcard domain", func(t *testing.T) {
		domains := []string{"*.example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cert.Cert.DNSNames[0] != "*.example.com" {
			t.Errorf("expected wildcard DNS name '*.example.com', got '%s'", cert.Cert.DNSNames[0])
		}
	})

	t.Run("certificate validity period", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		now := time.Now()
		if cert.NotBefore.After(now) {
			t.Errorf("expected NotBefore to be before or equal to now, got %v", cert.NotBefore)
		}

		expectedExpiry := now.Add(365 * 24 * time.Hour)
		timeDiff := cert.NotAfter.Sub(expectedExpiry)
		if timeDiff > time.Minute || timeDiff < -time.Minute {
			t.Errorf("expected NotAfter to be approximately 1 year from now, got %v", cert.NotAfter)
		}

		notBeforeDiff := cert.Cert.NotBefore.Sub(cert.NotBefore)
		if notBeforeDiff > time.Second || notBeforeDiff < -time.Second {
			t.Errorf("expected x509 NotBefore to be close to Certificate NotBefore, diff: %v", notBeforeDiff)
		}

		notAfterDiff := cert.Cert.NotAfter.Sub(cert.NotAfter)
		if notAfterDiff > time.Second || notAfterDiff < -time.Second {
			t.Errorf("expected x509 NotAfter to be close to Certificate NotAfter, diff: %v", notAfterDiff)
		}
	})

	t.Run("certificate has required key usages", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedKeyUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
		if cert.Cert.KeyUsage != expectedKeyUsage {
			t.Errorf("expected KeyUsage %v, got %v", expectedKeyUsage, cert.Cert.KeyUsage)
		}

		if len(cert.Cert.ExtKeyUsage) != 1 {
			t.Fatalf("expected 1 ExtKeyUsage, got %d", len(cert.Cert.ExtKeyUsage))
		}

		if cert.Cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
			t.Errorf("expected ExtKeyUsage ServerAuth, got %v", cert.Cert.ExtKeyUsage[0])
		}
	})

	t.Run("certificate organization", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(cert.Cert.Subject.Organization) != 1 {
			t.Fatalf("expected 1 organization, got %d", len(cert.Cert.Subject.Organization))
		}

		if cert.Cert.Subject.Organization[0] != "Balance Load Balancer" {
			t.Errorf("expected organization 'Balance Load Balancer', got '%s'", cert.Cert.Subject.Organization[0])
		}
	})

	t.Run("certificate has valid serial number", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cert.Cert.SerialNumber == nil {
			t.Fatal("expected serial number, got nil")
		}

		if cert.Cert.SerialNumber.Sign() <= 0 {
			t.Errorf("expected positive serial number, got %v", cert.Cert.SerialNumber)
		}
	})

	t.Run("TLSCert is populated correctly", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(cert.TLSCert.Certificate) != 1 {
			t.Fatalf("expected 1 certificate in TLSCert, got %d", len(cert.TLSCert.Certificate))
		}

		if cert.TLSCert.PrivateKey == nil {
			t.Fatal("expected private key in TLSCert, got nil")
		}

		parsedCert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse TLSCert certificate: %v", err)
		}

		if parsedCert.Subject.CommonName != "example.com" {
			t.Errorf("expected CommonName 'example.com' in TLSCert, got '%s'", parsedCert.Subject.CommonName)
		}
	})

	t.Run("unique serial numbers", func(t *testing.T) {
		domains := []string{"example.com"}

		cert1, err := GenerateSelfSignedCertificate(domains)
		if err != nil {
			t.Fatalf("expected no error for first cert, got %v", err)
		}

		cert2, err := GenerateSelfSignedCertificate(domains)
		if err != nil {
			t.Fatalf("expected no error for second cert, got %v", err)
		}

		if cert1.Cert.SerialNumber.Cmp(cert2.Cert.SerialNumber) == 0 {
			t.Error("expected unique serial numbers for different certificates")
		}
	})

	t.Run("basic constraints valid", func(t *testing.T) {
		domains := []string{"example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !cert.Cert.BasicConstraintsValid {
			t.Error("expected BasicConstraintsValid to be true")
		}
	})
}
