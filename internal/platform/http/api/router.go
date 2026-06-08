package httpapi

import (
	"github-release-notifier/internal/platform/http/middleware"
	appmetrics "github-release-notifier/internal/platform/metrics"

	"github.com/gin-gonic/gin"
)

func NewRouter(subscriptionHandler *SubscriptionHandler, metricSet *appmetrics.Metrics) *gin.Engine {
	router := gin.New()
	router.Use(middleware.RequestMetricsMiddleware(metricSet))
	router.Use(middleware.RequestLoggerMiddleware())
	router.Use(gin.Recovery())
	router.GET("/metrics", gin.WrapH(metricSet.Handler()))

	api := router.Group("/api")
	api.POST("/subscribe", subscriptionHandler.Create)
	api.GET("/subscriptions", subscriptionHandler.List)
	api.GET("/confirm/:token", subscriptionHandler.Confirm)
	api.GET("/unsubscribe/:token", subscriptionHandler.Cancel)

	return router
}
