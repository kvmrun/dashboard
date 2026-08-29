package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/0xef53/kvmrun-dashboard/internal/auth"
)

// Auth holds the dependencies of the login/logout handlers.
type Auth struct {
	PAM       *auth.PAM
	Sessions  *auth.SessionStore
	LoginTmpl *template.Template
	Cookie    string
	TTL       time.Duration
}

var authLogger = log.StandardLogger().WithField("subsystem", "auth")

// LoginPage renders the login form (GET /login).
func (a *Auth) LoginPage(c *gin.Context) {
	a.renderLogin(c, "")
}

// Login verifies the posted credentials via PAM and starts a session
// (POST /login).
func (a *Auth) Login(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	if username == "" || password == "" {
		a.renderLogin(c, "Please enter both username and password.")
		return
	}

	if err := a.PAM.Authenticate(username, password); err != nil {
		authLogger.WithField("user", username).WithError(err).Warn("PAM authentication failed")
		a.renderLogin(c, "Invalid username or password.")
		return
	}

	id, err := a.Sessions.Create(username)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Enable Secure when the dashboard is served over TLS/HTTPS.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(a.Cookie, id, int(a.TTL.Seconds()), "/", "", false, true)
	authLogger.WithField("user", username).Info("user logged in")
	c.Redirect(http.StatusSeeOther, "/")
}

// Logout destroys the session and clears the cookie (POST /logout).
func (a *Auth) Logout(c *gin.Context) {
	if id, err := c.Cookie(a.Cookie); err == nil {
		a.Sessions.Delete(id)
	}
	c.SetCookie(a.Cookie, "", -1, "/", "", false, true)
	c.Redirect(http.StatusSeeOther, "/login")
}

func (a *Auth) renderLogin(c *gin.Context, errMsg string) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := a.LoginTmpl.Execute(c.Writer, gin.H{"Error": errMsg}); err != nil {
		c.Error(err)
	}
}
