// Package middleware contains the Gin middleware used by the dashboard
// HTTP server.
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

var logger = log.StandardLogger().WithField("subsystem", "http")

// Logging logs each request with method, path, status and duration.
func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.WithFields(log.Fields{
			"method":   c.Request.Method,
			"path":     c.Request.URL.Path,
			"status":   c.Writer.Status(),
			"duration": time.Since(start).String(),
			"remote":   c.ClientIP(),
		}).Info("request")
	}
}
