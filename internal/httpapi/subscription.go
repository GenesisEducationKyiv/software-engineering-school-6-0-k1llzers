package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/mail"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/readmodel"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type subscriptionService interface {
	Subscribe(ctx context.Context, email string, repositoryFullName string) error
	ConfirmSubscription(ctx context.Context, token string) error
	CancelSubscription(ctx context.Context, token string) error
	ListSubscriptions(ctx context.Context, email string) ([]readmodel.SubscriptionView, error)
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
		switch {
		case errors.Is(err, domain.ErrIncorrectRepositoryFormat):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, domain.ErrAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, domain.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.Status(http.StatusOK)
}

func (h *SubscriptionHandler) List(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}

	if _, err := mail.ParseAddress(email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}

	subscriptions, err := h.subscriptions.ListSubscriptions(c.Request.Context(), email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	response := make([]listSubscriptionsResponse, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		response = append(response, listSubscriptionsResponse{
			Email:       subscription.Email,
			Repo:        subscription.Repo,
			Confirmed:   subscription.Confirmed,
			LastSeenTag: subscription.LastSeenTag,
		})
	}

	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) Confirm(c *gin.Context) {
	token := c.Param("token")
	if _, err := uuid.Parse(token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid confirmation token"})
		return
	}

	if err := h.subscriptions.ConfirmSubscription(c.Request.Context(), token); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.Status(http.StatusOK)
}

func (h *SubscriptionHandler) Cancel(c *gin.Context) {
	token := c.Param("token")
	if _, err := uuid.Parse(token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cancellation token"})
		return
	}

	if err := h.subscriptions.CancelSubscription(c.Request.Context(), token); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.Status(http.StatusOK)
}
