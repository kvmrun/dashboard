package cert

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// TestNewClientConfig verifies that the embedded agent PKI is present in the
// binary and loads into a usable client config: the client key/cert pair
// parses and the cert is issued by the embedded CA and usable for client
// auth (the way the guest agent checks it).
func TestNewClientConfig(t *testing.T) {
	cfg, err := NewClientConfig(EmbedStore)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs is nil")
	}

	clientCert := cfg.Certificates[0].Leaf
	if clientCert == nil {
		// Leaf is not populated for embedded certs — parse it ourselves.
		// client.crt may contain the cert concatenated with the CA, so
		// take the first CERTIFICATE PEM block.
		var b *pem.Block
		for rest := cfg.Certificates[0].Certificate[0]; ; {
			var next *pem.Block
			next, rest = pem.Decode(rest)
			if next == nil {
				break
			}
			if b == nil && next.Type == "CERTIFICATE" {
				b = next
			}
		}
		certs, err := x509.ParseCertificates(b.Bytes)
		if err != nil {
			t.Fatalf("parse client cert: %v", err)
		}
		clientCert = certs[0]
	}

	for _, name := range []string{"CA.crt", "client.crt", "client.key"} {
		if _, err := EmbedStore.ReadFile(name); err != nil {
			t.Errorf("EmbedStore.ReadFile(%q): %v", name, err)
		}
	}
	caPEM, err := EmbedStore.ReadFile("CA.crt")
	if err != nil {
		t.Fatalf("read CA: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to append CA.crt")
	}
	if _, err := clientCert.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Errorf("client cert not accepted as a client cert by the embedded CA: %v", err)
	}
}
