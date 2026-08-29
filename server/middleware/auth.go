package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/0xef53/kvmrun-dashboard/internal/auth"
)

// UserKey is the gin context key under which the authenticated username
// is stored (via c.Get(UserKey) in downstream handlers).
const UserKey = "user"

// RequireAuth requires a valid session cookie. Page requests without a
// session are redirected to the login page; API requests get a 401.
//
// Public routes (login page, static assets) must be registered on the
// engine before this middleware is added, since gin binds the handler
// chain at route-registration time.
func RequireAuth(store *auth.SessionStore, cookieName, loginPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := c.Cookie(cookieName)
		if err == nil {
			if sess, ok := store.Get(id); ok {
				c.Set(UserKey, sess.Username)
				c.Next()
				return
			}
		}
		if isAPIRequest(c) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Redirect(http.StatusSeeOther, loginPath)
		c.Abort()
	}
}

func isAPIRequest(c *gin.Context) bool {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		return true
	}
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}
