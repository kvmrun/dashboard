// Command dashboard is the web UI for the kvmrund daemon.
//
// It serves a browser dashboard that visualizes the same information that
// the vmm CLI exposes and lets the user perform basic operations (list,
// inspect, start/stop) over HTTP. All data comes from the kvmrund daemon
// via its gRPC interface — the dashboard itself stores no state.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/0xef53/kvmrun-dashboard/internal/auth"
	"github.com/0xef53/kvmrun-dashboard/internal/config"
	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
	"github.com/0xef53/kvmrun-dashboard/server"
)

const version = "0.1.0"

var (
	pamService = flag.String("pam-service", "login",
		"PAM service name to authenticate logins against (/etc/pam.d/<name>)")
	sessionTTL = flag.Duration("session-ttl", 12*time.Hour,
		"how long a login session stays valid")
	cookieName = flag.String("cookie-name", "kvmrun-dashboard-session",
		"name of the session cookie")
)

func main() {
	cfg := config.Default()

	flag.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "address to serve the dashboard on")
	flag.StringVar(&cfg.DaemonAddr, "daemon", cfg.DaemonAddr, "kvmrund daemon address (host:port or unix:@abstract-socket)")
	flag.StringVar(&cfg.CertDir, "cert-dir", cfg.CertDir, "directory with the kvmrun TLS certificates (client.crt, client.key)")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "dashboard %s: %v\n", version, err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	logger := log.NewEntry(log.StandardLogger()).WithField("subsystem", "dashboard")

	tlsConfig, err := cfg.TLSConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Same fallback as the vmm CLI: no certificates on this host
			// means the connection goes out unencrypted.
			logger.Warn("kvmrun TLS certificates not found, connecting to kvmrund without TLS")
			tlsConfig = nil
		} else {
			return err
		}
	}

	daemonClient, err := daemon.New(cfg.DaemonAddr, tlsConfig)
	if err != nil {
		return err
	}
	defer daemonClient.Close()

	srv := server.New(server.Config{
		Daemon:     daemonClient,
		PAM:        auth.NewPAM(*pamService),
		Sessions:   auth.NewSessionStore(*sessionTTL),
		CookieName: *cookieName,
		SessionTTL: *sessionTTL,
	})
	logger.Info("dashboard is ready")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Listen(ctx, cfg.ListenAddr)
	}()

	select {
	case <-ctx.Done():
		// Listen() shuts the server down gracefully once ctx is done.
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
