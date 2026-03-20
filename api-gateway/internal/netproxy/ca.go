// Package netproxy implements Layer 3: a transparent HTTPS MITM proxy that
// intercepts outbound connections to known LLM API domains, records spans,
// and optionally substitutes virtual keys with real keys from the vault.
//
// Clients configure their HTTP_PROXY / HTTPS_PROXY env vars to point at
// http://localhost:8443.  The proxy handles HTTP CONNECT tunnelling; for
// known LLM hosts it terminates TLS with a locally-generated certificate
// (signed by the per-process CA below) and inspects the plaintext request.
// All other hosts are forwarded transparently without inspection.
package netproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"time"
)

// CA is an in-memory certificate authority used to sign per-host leaf certs
// for HTTPS interception.  It is generated fresh on every process start.
// Users must install the CA cert into their OS trust store once using
// GET /api/v1/netproxy/ca.crt + scripts/install-ca-cert.sh.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte // PEM-encoded CA cert for the /ca.crt endpoint

	mu    sync.Mutex
	cache map[string]*tls.Certificate // hostname → signed leaf cert
}

// NewCA generates a fresh ECDSA P-256 CA keypair and a self-signed certificate.
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
			Organization: []string{"AgentFabric NetProxy"},
			CommonName:   "AgentFabric NetProxy CA",
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

	return &CA{
		cert:    cert,
		key:     key,
		certPEM: certPEM,
		cache:   make(map[string]*tls.Certificate),
	}, nil
}

// CertPEM returns the CA certificate in PEM format.
// Write this directly to a .crt file and add it to your OS trust store.
// Safe for concurrent use.
func (ca *CA) CertPEM() []byte { return ca.certPEM }

// TLSConfigFor returns a *tls.Config that presents a dynamically-generated
// leaf certificate for hostname, signed by this CA.  Leaf certs are cached
// indefinitely for the lifetime of the process.
// Safe for concurrent use.
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
	// IP addresses require IPAddresses SAN; hostnames use DNSNames SAN.
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{hostname}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &leafKey.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}

	// Build tls.Certificate from DER + key (avoids PEM round-trip).
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
