package httpapi

import (
	"errors"
	"net/http"
	"net/mail"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/readmodel"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func writeSubscriptionCreateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrIncorrectRepositoryFormat):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrAlreadyExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrRateLimited):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "github is temporarily unavailable, please try again later"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func writeSubscriptionConfirmError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidToken):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func writeSubscriptionCancelError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func parseRequiredEmail(email string) error {
	if email == "" {
		return errors.New("email is required")
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return errors.New("invalid email")
	}

	return nil
}

func parseToken(raw string, invalidMessage string) (string, error) {
	if _, err := uuid.Parse(raw); err != nil {
		return "", errors.New(invalidMessage)
	}

	return raw, nil
}

func toListSubscriptionsResponse(subscriptions []readmodel.SubscriptionView) []listSubscriptionsResponse {
	response := make([]listSubscriptionsResponse, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		response = append(response, listSubscriptionsResponse{
			Email:       subscription.Email,
			Repo:        subscription.Repo,
			Confirmed:   subscription.Confirmed,
			LastSeenTag: subscription.LastSeenTag,
		})
	}

	return response
}
