// Package handlers contains the dashboard's HTTP handlers. Handlers are
// grouped by the kvmrun domain they operate on (machines, storage,
// network, ...), mirroring the vmm subcommand groups.
package handlers

import (
	"html/template"

	"github.com/gin-gonic/gin"

	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
	"github.com/0xef53/kvmrun-dashboard/internal/version"
	"github.com/0xef53/kvmrun-dashboard/server/middleware"
)

// Handlers holds the dependencies shared by all route handlers.
type Handlers struct {
	Daemon *daemon.Client
	Pages  map[string]*template.Template
}

// render sends a rendered page to the client. page is the key of the page
// template (e.g. "machines.html"); each page template is a separate set
// parsed together with the shared layout. The authenticated username (set
// by middleware.RequireAuth) is injected so the layout nav can show it,
// together with the version strings for the header chips.
func (h *Handlers) render(c *gin.Context, page string, status int, data gin.H) {
	data["User"] = c.GetString(middleware.UserKey)
	data["KvmrunVersion"] = version.Kvmrun
	data["DashboardVersion"] = version.Dashboard
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")

	if err := h.Pages[page].ExecuteTemplate(c.Writer, page, data); err != nil {
		c.Error(err)
	}
}
