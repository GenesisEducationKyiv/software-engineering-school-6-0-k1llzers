package httpapi

import "github.com/gin-gonic/gin"

func NewRouter(subscriptionHandler *SubscriptionHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api")
	api.POST("/subscribe", subscriptionHandler.Create)
	api.GET("/subscriptions", subscriptionHandler.List)
	api.GET("/confirm/:token", subscriptionHandler.Confirm)
	api.GET("/unsubscribe/:token", subscriptionHandler.Cancel)

	return router
}
