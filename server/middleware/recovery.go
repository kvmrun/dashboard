package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recovery converts panics in handlers into 500 responses and logs them.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.WithField("panic", r).Error("recovered from panic")
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
