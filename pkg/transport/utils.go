package transport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
)

// GenerateSelfSignedCertificate generates a self-signed certificate for the given domains.
// It creates a new RSA 2048-bit private key and a certificate valid for 365 days.
// The certificate is configured for server authentication and includes the provided domains as DNS names.
func GenerateSelfSignedCertificate(domains []string) (*Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	start := time.Now()
	end := start.Add(365 * 24 * time.Hour)

	serialNum, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNum,
		Subject: pkix.Name{
			Organization: []string{"Balance Load Balancer"},
			CommonName:   domains[0],
		},
		NotBefore:             start,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              domains,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	return &Certificate{
		Cert:      cert,
		TLSCert:   tlsCert,
		Domains:   domains,
		NotBefore: start,
		NotAfter:  end,
	}, nil
}

// SaveCertificateToPEM saves the provided certificate and private key to the specified files in PEM format.
// It performs the file writing operations concurrently. If any error occurs during writing,
// it attempts to remove any partially created files.
func SaveCertificateToPEM(cert *Certificate, certFile, keyFile string) error {
	var g errgroup.Group

	g.Go(func() error {
		return saveCertificate(cert, certFile)
	})

	g.Go(func() error {
		return savePrimaryKey(cert, keyFile)
	})

	if err := g.Wait(); err != nil {
		_ = os.Remove(certFile)
		_ = os.Remove(keyFile)
		return fmt.Errorf("failed to save certificate or key: %w", err)
	}

	return nil
}

// saveCertificate writes the certificate data to a file in PEM format.
func saveCertificate(cert *Certificate, certFile string) error {
	certOut, err := os.Create(certFile)
	if err != nil {
		return fmt.Errorf("failed to create certificate file: %w", err)
	}
	defer certOut.Close()

	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.TLSCert.Certificate[0],
	}

	if err := pem.Encode(certOut, block); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	return nil
}

// savePrimaryKey writes the RSA private key to a file in PEM format.
// It expects the private key in the certificate to be of type *rsa.PrivateKey.
func savePrimaryKey(cert *Certificate, keyfile string) error {
	keyOut, err := os.Create(keyfile)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyOut.Close()

	privateKey, ok := cert.TLSCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("private key is not RSA")
	}

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err := pem.Encode(keyOut, block); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	return nil
}
