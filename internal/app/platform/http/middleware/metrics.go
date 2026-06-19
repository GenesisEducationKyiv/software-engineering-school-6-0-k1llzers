package middleware

import (
	"time"

	appmetrics "github-release-notifier/internal/app/platform/metrics"

	"github.com/gin-gonic/gin"
)

func RequestMetricsMiddleware(metricSet *appmetrics.Metrics) gin.HandlerFunc {
	if metricSet == nil {
		panic("metrics is required")
	}

	return func(c *gin.Context) {
		if currentRoute(c) == "/metrics" {
			c.Next()
			return
		}

		startedAt := time.Now()
		c.Next()

		metricSet.ObserveHTTPRequest(
			c.Request.Context(),
			c.Request.Method,
			currentRoute(c),
			c.Writer.Status(),
			time.Since(startedAt),
		)
	}
}
