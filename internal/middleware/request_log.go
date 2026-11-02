package middleware

import (
	"crypto/rand"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLog assigns a server-generated ID and logs metadata only.
func RequestLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := rand.Text()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "<unmatched>"
		}
		logger.Info("http request",
			"request_id", requestID,
			"method", c.Request.Method,
			"route", route,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
