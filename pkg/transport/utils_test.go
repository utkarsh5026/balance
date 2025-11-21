package transport

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
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

func TestSaveCertificateToPEM(t *testing.T) {
	t.Run("successfully saves certificate and key", func(t *testing.T) {
		cert, err := GenerateSelfSignedCertificate([]string{"example.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		err = SaveCertificateToPEM(cert, certFile, keyFile)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			t.Error("certificate file was not created")
		}

		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			t.Error("key file was not created")
		}
	})

	t.Run("saved certificate is valid PEM", func(t *testing.T) {
		cert, err := GenerateSelfSignedCertificate([]string{"test.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		err = SaveCertificateToPEM(cert, certFile, keyFile)
		if err != nil {
			t.Fatalf("failed to save certificate: %v", err)
		}

		certData, err := os.ReadFile(certFile)
		if err != nil {
			t.Fatalf("failed to read certificate file: %v", err)
		}

		block, _ := pem.Decode(certData)
		if block == nil {
			t.Fatal("failed to decode PEM block from certificate file")
		}

		if block.Type != "CERTIFICATE" {
			t.Errorf("expected PEM type 'CERTIFICATE', got '%s'", block.Type)
		}

		parsedCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("failed to parse certificate from PEM: %v", err)
		}

		if parsedCert.Subject.CommonName != "test.com" {
			t.Errorf("expected CommonName 'test.com', got '%s'", parsedCert.Subject.CommonName)
		}
	})

	t.Run("saved key is valid PEM", func(t *testing.T) {
		cert, err := GenerateSelfSignedCertificate([]string{"test.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		err = SaveCertificateToPEM(cert, certFile, keyFile)
		if err != nil {
			t.Fatalf("failed to save certificate: %v", err)
		}

		keyData, err := os.ReadFile(keyFile)
		if err != nil {
			t.Fatalf("failed to read key file: %v", err)
		}

		block, _ := pem.Decode(keyData)
		if block == nil {
			t.Fatal("failed to decode PEM block from key file")
		}

		if block.Type != "RSA PRIVATE KEY" {
			t.Errorf("expected PEM type 'RSA PRIVATE KEY', got '%s'", block.Type)
		}

		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			t.Fatalf("failed to parse private key from PEM: %v", err)
		}

		if privateKey.N == nil {
			t.Error("private key modulus is nil")
		}
	})

	t.Run("saved certificate can be loaded and used", func(t *testing.T) {
		originalCert, err := GenerateSelfSignedCertificate([]string{"example.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		err = SaveCertificateToPEM(originalCert, certFile, keyFile)
		if err != nil {
			t.Fatalf("failed to save certificate: %v", err)
		}

		loadedTLSCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			t.Fatalf("failed to load certificate: %v", err)
		}

		if len(loadedTLSCert.Certificate) != 1 {
			t.Fatalf("expected 1 certificate, got %d", len(loadedTLSCert.Certificate))
		}

		loadedX509Cert, err := x509.ParseCertificate(loadedTLSCert.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse loaded certificate: %v", err)
		}

		if loadedX509Cert.Subject.CommonName != "example.com" {
			t.Errorf("expected CommonName 'example.com', got '%s'", loadedX509Cert.Subject.CommonName)
		}

		if loadedTLSCert.PrivateKey == nil {
			t.Fatal("expected private key, got nil")
		}

		_, ok := loadedTLSCert.PrivateKey.(*rsa.PrivateKey)
		if !ok {
			t.Error("expected RSA private key")
		}
	})

	t.Run("fails with invalid certificate file path", func(t *testing.T) {
		cert, err := GenerateSelfSignedCertificate([]string{"example.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		invalidPath := filepath.Join("nonexistent", "directory", "cert.pem")
		keyFile := filepath.Join(t.TempDir(), "key.pem")

		err = SaveCertificateToPEM(cert, invalidPath, keyFile)
		if err == nil {
			t.Error("expected error with invalid certificate path, got nil")
		}
	})

	t.Run("fails with invalid key file path", func(t *testing.T) {
		cert, err := GenerateSelfSignedCertificate([]string{"example.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		certFile := filepath.Join(t.TempDir(), "cert.pem")
		invalidPath := filepath.Join("nonexistent", "directory", "key.pem")

		err = SaveCertificateToPEM(cert, certFile, invalidPath)
		if err == nil {
			t.Error("expected error with invalid key path, got nil")
		}

		if _, err := os.Stat(certFile); err == nil {
			t.Error("certificate file should have been cleaned up after error")
		}
	})

	t.Run("cleans up on failure", func(t *testing.T) {
		cert, err := GenerateSelfSignedCertificate([]string{"example.com"})
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		invalidKeyPath := filepath.Join("nonexistent", "directory", "key.pem")

		err = SaveCertificateToPEM(cert, certFile, invalidKeyPath)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if _, err := os.Stat(certFile); !os.IsNotExist(err) {
			t.Error("certificate file should have been removed after failure")
		}
	})

	t.Run("preserves certificate domains", func(t *testing.T) {
		domains := []string{"example.com", "www.example.com", "api.example.com"}
		cert, err := GenerateSelfSignedCertificate(domains)
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		err = SaveCertificateToPEM(cert, certFile, keyFile)
		if err != nil {
			t.Fatalf("failed to save certificate: %v", err)
		}

		certData, err := os.ReadFile(certFile)
		if err != nil {
			t.Fatalf("failed to read certificate file: %v", err)
		}

		block, _ := pem.Decode(certData)
		parsedCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("failed to parse certificate: %v", err)
		}

		if len(parsedCert.DNSNames) != 3 {
			t.Errorf("expected 3 DNS names, got %d", len(parsedCert.DNSNames))
		}

		for i, expected := range domains {
			if parsedCert.DNSNames[i] != expected {
				t.Errorf("expected DNS name '%s' at index %d, got '%s'", expected, i, parsedCert.DNSNames[i])
			}
		}
	})
}
