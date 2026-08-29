// Package config holds the dashboard's runtime configuration: where the
// HTTP server listens and how to reach the kvmrund daemon.
package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
)

// Default values, kept in sync with the kvmrun daemon (kvmrun/internal/appconf).
const (
	DefaultListenAddr = ":8080"
	DefaultDaemonAddr = "unix:@/run/kvmrund.sock"
	DefaultCertDir    = "/usr/share/kvmrun/tls"
)

// Config is the full set of dashboard settings.
type Config struct {
	// ListenAddr is the address the dashboard HTTP server binds to.
	ListenAddr string
	// DaemonAddr is the kvmrund address: host:port or an abstract/unix
	// socket path (e.g. "unix:@/run/kvmrund.sock").
	DaemonAddr string
	// CertDir contains the kvmrun TLS certificates (client.crt, client.key).
	CertDir string
}

// Default returns the configuration with the same defaults the vmm CLI
// uses.
func Default() Config {
	return Config{
		ListenAddr: DefaultListenAddr,
		DaemonAddr: DefaultDaemonAddr,
		CertDir:    DefaultCertDir,
	}
}

// TLSConfig builds the mutual TLS configuration used to talk to kvmrund,
// from the client certificate in CertDir. The scheme is the same the vmm
// CLI uses (client.crt holds the client cert concatenated with the CA).
func (c Config) TLSConfig() (*tls.Config, error) {
	certFile := filepath.Join(c.CertDir, "client.crt")
	keyFile := filepath.Join(c.CertDir, "client.key")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	if len(cert.Certificate) != 2 {
		return nil, errors.New("certificate should contain 2 concatenated certificates: cert + CA")
	}

	ca, err := x509.ParseCertificate(cert.Certificate[1])
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(ca)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		RootCAs:      certPool,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
