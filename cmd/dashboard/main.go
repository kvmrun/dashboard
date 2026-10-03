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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/0xef53/kvmrun-dashboard/internal/auth"
	"github.com/0xef53/kvmrun-dashboard/internal/config"
	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
	"github.com/0xef53/kvmrun-dashboard/server"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
}

func main() {
	app := new(cli.Command)

	app.Usage = "WEB interface for managing virtual machines"
	app.Action = run

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "address to serve the dashboard on",
			Value: config.DefaultListenAddr,
		},
		&cli.StringFlag{
			Name:  "daemon",
			Usage: "kvmrund daemon address (host:port or unix:@abstract-socket)",
			Value: config.DefaultDaemonAddr,
		},
		&cli.StringFlag{
			Name:  "cert-dir",
			Usage: "directory with the kvmrun TLS certificates (client.crt, client.key)",
			Value: config.DefaultCertDir,
		},
		&cli.StringFlag{
			Name:  "pam-service",
			Usage: "PAM service name to authenticate logins against (/etc/pam.d/<name>)",
			Value: "login",
		},
		&cli.DurationFlag{
			Name:  "session-ttl",
			Usage: "how long a login session stays valid",
			Value: 12 * time.Hour,
		},
		&cli.StringFlag{
			Name:  "cookie-name",
			Usage: "name of the session cookie",
			Value: "kvmrun-dashboard-session",
		},
		&cli.BoolFlag{
			Name:    "debug",
			Usage:   "print debug information",
			Sources: cli.EnvVars("KVMRUND_DEBUG", "DEBUG"),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatalln(err)
	}
}

func run(ctx context.Context, c *cli.Command) error {
	if c.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}

	cfg := config.Config{
		ListenAddr: c.String("listen"),
		DaemonAddr: c.String("daemon"),
		CertDir:    c.String("cert-dir"),
	}

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
		PAM:        auth.NewPAM(c.String("pam-service")),
		Sessions:   auth.NewSessionStore(c.Duration("session-ttl")),
		CookieName: c.String("cookie-name"),
		SessionTTL: c.Duration("session-ttl"),
	})
	logger.Info("dashboard is ready")

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
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
