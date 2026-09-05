// Package server implements the dashboard's HTTP layer: the Gin engine,
// route registration, template loading and static file serving.
package server

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0xef53/kvmrun-dashboard/internal/auth"
	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
	"github.com/0xef53/kvmrun-dashboard/server/handlers"
	"github.com/0xef53/kvmrun-dashboard/server/middleware"
	"github.com/0xef53/kvmrun-dashboard/server/templates"
	"github.com/0xef53/kvmrun-dashboard/web/novnc"
	"github.com/0xef53/kvmrun-dashboard/web/static"
	"github.com/0xef53/kvmrun-dashboard/web/xterm"
)

// Config carries everything the HTTP layer needs.
type Config struct {
	Daemon     *daemon.Client
	PAM        *auth.PAM
	Sessions   *auth.SessionStore
	CookieName string
	SessionTTL time.Duration
}

// Server bundles the HTTP server with its dependencies.
type Server struct {
	httpServer *http.Server
}

// New builds the Gin engine, loads the templates and registers all routes.
//
// Routes registered before the RequireAuth middleware (login, static
// assets, health check) are public; everything after it requires a valid
// session. Gin binds the handler chain at registration time, so the
// ordering below is load-bearing.
func New(cfg Config) *Server {
	gin.SetMode(gin.ReleaseMode)

	// Each page is parsed together with the shared layout into its own
	// template set, so the "content" block of different pages never collide.
	pages := make(map[string]*template.Template, 4)
	for _, name := range []string{"machines.html", "system.html", "tasks.html", "top.html"} {
		pages[name] = template.Must(template.New(name).ParseFS(templates.FS, "layout.html", name))
	}
	loginTmpl := template.Must(template.New("login.html").ParseFS(templates.FS, "login.html"))

	engine := gin.New()
	engine.Use(middleware.Recovery(), middleware.Logging())

	// Embedded frontend assets (CSS/JS) — the binary is self-contained.
	engine.GET(
		"/static/*filepath",
		gin.WrapH(http.StripPrefix("/static/", http.FileServer(http.FS(static.FS)))),
	)

	h := &handlers.Handlers{
		Daemon: cfg.Daemon,
		Pages:  pages,
	}
	a := &handlers.Auth{
		PAM:       cfg.PAM,
		Sessions:  cfg.Sessions,
		LoginTmpl: loginTmpl,
		Cookie:    cfg.CookieName,
		TTL:       cfg.SessionTTL,
	}

	// Public routes — registered before RequireAuth, since gin binds the
	// handler chain at route-registration time.
	engine.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.GET("/login", a.LoginPage)
	engine.POST("/login", a.Login)

	// Everything below requires a PAM-authenticated session.
	engine.Use(middleware.RequireAuth(cfg.Sessions, cfg.CookieName, "/login"))

	// Server-rendered pages and actions.
	engine.POST("/logout", a.Logout)
	engine.GET("/", h.SystemIndex)
	engine.GET("/machines", h.MachinesList)
	engine.GET("/machines/:name", h.MachineDetail)
	engine.GET("/tasks", h.TasksIndex)
	engine.GET("/top", h.TopIndex)

	// Embedded noVNC client (v1.7.0) for the VNC console on the machine
	// detail page. It reaches the VM's VNC server through the
	// /api/v1/machines/{name}/vnc-ws proxy below.
	engine.GET("/novnc/*filepath",
		gin.WrapH(http.StripPrefix("/novnc/", http.FileServer(http.FS(novnc.FS)))),
	)

	// Embedded xterm.js terminal for the agent built-in SSH console on the
	// machine detail page. It talks to the
	// /api/v1/machines/{name}/ssh-ws proxy below.
	engine.GET("/xterm/*filepath",
		gin.WrapH(http.StripPrefix("/xterm/", http.FileServer(http.FS(xterm.FS)))),
	)

	engine.POST("/machines/:name/start", h.StartMachine)
	engine.POST("/machines/:name/stop", h.StopMachine)
	engine.POST("/machines/:name/restart", h.RestartMachine)
	engine.POST("/machines/:name/reset", h.ResetMachine)

	// JSON API consumed by the frontend (web/static/js/app.js).
	api := engine.Group("/api/v1")
	{
		api.GET("/system", h.SystemJSON)
		api.GET("/machines", h.MachinesListJSON)
		api.GET("/machines/:name", h.MachineDetailJSON)
		api.GET("/machines/:name/networks", h.MachineNetworksJSON)
		api.GET("/machines/:name/comment", h.MachineCommentJSON)
		api.PUT("/machines/:name/comment", h.UpdateMachineCommentJSON)
		api.GET("/tasks", h.TasksListJSON)
		api.POST("/machines/:name/vnc", h.VNCActivateJSON)
		// WebSocket proxy (mini websockify) between the embedded noVNC
		// client and the VM's local VNC server.
		api.GET("/machines/:name/vnc-ws", h.VNCProxyWS)
		// WebSocket proxy between the embedded xterm.js terminal and the
		// guest agent's built-in SSH server over AF_VSOCK.
		api.GET("/machines/:name/ssh-ws", h.SSHProxyWS)
		// TODO: the remaining domains once their daemon services are wired up:
		//   api.GET("/machines/:name/disks", h.DisksListJSON)   // storage
		//   api.GET("/network", h.NetworkListJSON)              // network
	}

	return &Server{
		httpServer: &http.Server{Handler: engine},
	}
}

// Listen serves HTTP on addr until ctx is done, then shuts the server
// down gracefully.
func (s *Server) Listen(ctx context.Context, addr string) error {
	s.httpServer.Addr = addr

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdownCtx)
	}()

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server, waiting for in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
