package httpapi

import "github.com/gin-gonic/gin"

func NewRouter(subscriptionHandler *SubscriptionHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.POST("/subscribe", subscriptionHandler.Create)
	router.GET("/subscriptions", subscriptionHandler.List)
	router.GET("/confirm/:token", subscriptionHandler.Confirm)
	router.GET("/unsubscribe/:token", subscriptionHandler.Cancel)

	return router
}
