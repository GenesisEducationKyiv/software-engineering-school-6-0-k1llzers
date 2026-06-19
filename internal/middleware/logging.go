package middleware

import (
	"github-release-notifier/internal/logging"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentRoute(c) == "/metrics" {
			c.Next()
			return
		}

		startedAt := time.Now()
		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"route", currentRoute(c),
			"status", status,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"client_ip", c.ClientIP(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}

		slog.Log(c.Request.Context(), logging.LogLevelForHTTPStatus(status), "request completed", attrs...)
	}
}

func currentRoute(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}

	return c.Request.URL.Path
}
