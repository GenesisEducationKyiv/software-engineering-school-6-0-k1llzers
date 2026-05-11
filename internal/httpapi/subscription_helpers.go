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

type errorResponseRule struct {
	match   func(error) bool
	status  int
	message func(error) string
}

type errorResponder struct {
	rules          []errorResponseRule
	defaultStatus  int
	defaultMessage string
}

func newErrorResponder(rules []errorResponseRule, defaultStatus int, defaultMessage string) errorResponder {
	return errorResponder{
		rules:          rules,
		defaultStatus:  defaultStatus,
		defaultMessage: defaultMessage,
	}
}

func (r errorResponder) Write(c *gin.Context, err error) {
	for _, rule := range r.rules {
		if rule.match(err) {
			c.JSON(rule.status, gin.H{"error": rule.message(err)})
			return
		}
	}

	c.JSON(r.defaultStatus, gin.H{"error": r.defaultMessage})
}

func matchDomainError(target error) func(error) bool {
	return func(err error) bool {
		return errors.Is(err, target)
	}
}

func errorMessage(err error) string {
	return err.Error()
}

func staticMessage(message string) func(error) string {
	return func(error) string {
		return message
	}
}

var subscriptionCreateErrorResponder = newErrorResponder(
	[]errorResponseRule{
		{match: matchDomainError(domain.ErrIncorrectRepositoryFormat), status: http.StatusBadRequest, message: errorMessage},
		{match: matchDomainError(domain.ErrAlreadyExists), status: http.StatusConflict, message: errorMessage},
		{match: matchDomainError(domain.ErrNotFound), status: http.StatusNotFound, message: errorMessage},
		{match: matchDomainError(domain.ErrRateLimited), status: http.StatusServiceUnavailable, message: staticMessage("github is temporarily unavailable, please try again later")},
	},
	http.StatusInternalServerError,
	"internal server error",
)

var subscriptionConfirmErrorResponder = newErrorResponder(
	[]errorResponseRule{
		{match: matchDomainError(domain.ErrInvalidToken), status: http.StatusBadRequest, message: errorMessage},
		{match: matchDomainError(domain.ErrNotFound), status: http.StatusNotFound, message: errorMessage},
	},
	http.StatusInternalServerError,
	"internal server error",
)

var subscriptionCancelErrorResponder = newErrorResponder(
	[]errorResponseRule{
		{match: matchDomainError(domain.ErrNotFound), status: http.StatusNotFound, message: errorMessage},
	},
	http.StatusInternalServerError,
	"internal server error",
)

func writeSubscriptionCreateError(c *gin.Context, err error) {
	subscriptionCreateErrorResponder.Write(c, err)
}

func writeSubscriptionConfirmError(c *gin.Context, err error) {
	subscriptionConfirmErrorResponder.Write(c, err)
}

func writeSubscriptionCancelError(c *gin.Context, err error) {
	subscriptionCancelErrorResponder.Write(c, err)
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
