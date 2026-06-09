package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github-release-notifier/internal/app/subscriptions"
	"github-release-notifier/internal/platform/logging"

	"github.com/gin-gonic/gin"
)

type subscriptionService interface {
	Subscribe(ctx context.Context, email string, repositoryFullName string) error
	ConfirmSubscription(ctx context.Context, token string) error
	CancelSubscription(ctx context.Context, token string) error
	ListSubscriptions(ctx context.Context, email string) ([]subscriptions.SubscriptionView, error)
}

type SubscriptionHandler struct {
	subscriptions subscriptionService
}

type createSubscriptionRequest struct {
	Email              string `json:"email" binding:"required,email"`
	RepositoryFullName string `json:"repo" binding:"required"`
}

type listSubscriptionsResponse struct {
	Email       string `json:"email"`
	Repo        string `json:"repo"`
	Confirmed   bool   `json:"confirmed"`
	LastSeenTag string `json:"last_seen_tag"`
}

func NewSubscriptionHandler(subscriptions subscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{subscriptions: subscriptions}
}

func (h *SubscriptionHandler) Create(c *gin.Context) {
	var req createSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.subscriptions.Subscribe(c.Request.Context(), req.Email, req.RepositoryFullName)
	if err != nil {
		response := subscriptionCreateErrorResponder.Resolve(err)
		slog.Log(
			c.Request.Context(),
			logging.LogLevelForHTTPStatus(response.status),
			"subscription create failed",
			"status", response.status,
			"email", req.Email,
			"repo", req.RepositoryFullName,
			"error", err,
		)
		c.JSON(response.status, gin.H{"error": response.message})
		return
	}

	c.Status(http.StatusOK)
}

func (h *SubscriptionHandler) List(c *gin.Context) {
	email := c.Query("email")
	if err := parseRequiredEmail(email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	subscriptionsByEmail, err := h.subscriptions.ListSubscriptions(c.Request.Context(), email)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "subscription list failed", "email", email, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, toListSubscriptionsResponse(subscriptionsByEmail))
}

func (h *SubscriptionHandler) Confirm(c *gin.Context) {
	token, err := parseToken(c.Param("token"), "invalid confirmation token")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.subscriptions.ConfirmSubscription(c.Request.Context(), token); err != nil {
		response := subscriptionConfirmErrorResponder.Resolve(err)
		slog.Log(
			c.Request.Context(),
			logging.LogLevelForHTTPStatus(response.status),
			"subscription confirm failed",
			"status", response.status,
			"error", err,
		)
		c.JSON(response.status, gin.H{"error": response.message})
		return
	}

	c.Status(http.StatusOK)
}

func (h *SubscriptionHandler) Cancel(c *gin.Context) {
	token, err := parseToken(c.Param("token"), "invalid cancellation token")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.subscriptions.CancelSubscription(c.Request.Context(), token); err != nil {
		response := subscriptionCancelErrorResponder.Resolve(err)
		slog.Log(
			c.Request.Context(),
			logging.LogLevelForHTTPStatus(response.status),
			"subscription cancel failed",
			"status", response.status,
			"error", err,
		)
		c.JSON(response.status, gin.H{"error": response.message})
		return
	}

	c.Status(http.StatusOK)
}
