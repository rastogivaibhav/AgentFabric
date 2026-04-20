// Package netproxy implements Layer 3: a transparent HTTPS MITM proxy that
// intercepts outbound connections to known LLM API domains, records spans,
// and optionally substitutes virtual keys with real keys from the vault.
//
// Clients configure their HTTP_PROXY / HTTPS_PROXY env vars to point at
// http://localhost:8443. The proxy handles HTTP CONNECT tunnelling; for
// known LLM hosts it terminates TLS with a locally-generated certificate
// (signed by the CA below) and inspects the plaintext request.
// All other hosts are forwarded transparently without inspection.
package netproxy

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CA signs per-host leaf certs for HTTPS interception.
// In production this CA should be loaded from persisted PEM files so the
// installed client trust root survives restarts.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte

	mu    sync.Mutex
	cache map[string]*tls.Certificate // hostname -> signed leaf cert
}

// NewCA generates a fresh ECDSA P-256 CA keypair and self-signed certificate.
// This is appropriate for dev and tests. Production should prefer LoadCAFromFiles.
func NewCA() (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Govagn NetProxy"},
			CommonName:   "Govagn NetProxy CA",
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	return newCAFromMaterial(cert, key, certPEM)
}

// LoadCAFromFiles loads a persisted CA cert/key pair from PEM files.
func LoadCAFromFiles(certFile, keyFile string) (*CA, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read netproxy CA cert: %w", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read netproxy CA key: %w", err)
	}

	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		return nil, err
	}
	key, err := parsePrivateKeyPEM(keyPEM)
	if err != nil {
		return nil, err
	}

	return newCAFromMaterial(cert, key, certPEM)
}

// LoadOrCreateCA loads a persisted CA when the files exist, otherwise creates
// one and writes it to disk. This keeps local dev restarts stable without
// requiring operators to pre-provision a CA.
func LoadOrCreateCA(certFile, keyFile string) (*CA, error) {
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("both netproxy CA file paths are required")
	}

	certExists := fileExists(certFile)
	keyExists := fileExists(keyFile)
	if certExists != keyExists {
		return nil, fmt.Errorf("netproxy CA files must both exist or both be absent")
	}
	if certExists {
		return LoadCAFromFiles(certFile, keyFile)
	}

	ca, err := NewCA()
	if err != nil {
		return nil, err
	}
	if err := writeCAFiles(certFile, keyFile, ca.certPEM, ca.keyPEM()); err != nil {
		return nil, err
	}
	return ca, nil
}

func newCAFromMaterial(cert *x509.Certificate, key *ecdsa.PrivateKey, certPEM []byte) (*CA, error) {
	if cert == nil || key == nil {
		return nil, fmt.Errorf("certificate and key are required")
	}
	return &CA{
		cert:    cert,
		key:     key,
		certPEM: certPEM,
		cache:   make(map[string]*tls.Certificate),
	}, nil
}

// CertPEM returns the CA certificate in PEM format.
func (ca *CA) CertPEM() []byte { return ca.certPEM }

func (ca *CA) keyPEM() []byte {
	keyDER, err := x509.MarshalECPrivateKey(ca.key)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// TLSConfigFor returns a TLS config that presents a per-host leaf certificate
// signed by this CA. Leaf certs are cached for the lifetime of the process.
func (ca *CA) TLSConfigFor(hostname string) (*tls.Config, error) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	leaf, err := ca.leafForHost(hostname)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*leaf},
	}, nil
}

// leafForHost returns (and caches) a leaf cert for hostname.
// Must be called with ca.mu held.
func (ca *CA) leafForHost(hostname string) (*tls.Certificate, error) {
	if cached, ok := ca.cache[hostname]; ok {
		return cached, nil
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: hostname,
		},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{hostname}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &leafKey.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  leafKey,
	}
	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	tlsCert.Leaf = leaf

	ca.cache[hostname] = tlsCert
	return tlsCert, nil
}

func parseCertificatePEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid netproxy CA certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse netproxy CA certificate: %w", err)
	}
	return cert, nil
}

func parsePrivateKeyPEM(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("invalid netproxy CA private key PEM")
	}

	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse netproxy CA EC private key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse netproxy CA PKCS8 private key: %w", err)
		}
		ecdsaKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("netproxy CA private key must be ECDSA, got %T", key)
		}
		return ecdsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported netproxy CA private key type %q", block.Type)
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func writeCAFiles(certFile, keyFile string, certPEM, keyPEM []byte) error {
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return fmt.Errorf("netproxy CA material is empty")
	}
	if err := os.MkdirAll(filepath.Dir(certFile), 0o755); err != nil {
		return fmt.Errorf("create netproxy CA cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o755); err != nil {
		return fmt.Errorf("create netproxy CA key dir: %w", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return fmt.Errorf("write netproxy CA cert: %w", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write netproxy CA key: %w", err)
	}
	return nil
}

var _ crypto.Signer = (*ecdsa.PrivateKey)(nil)
