package httpapi

import (
	"errors"
	"net/http"
	"net/mail"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/readmodel"
	"github-release-notifier/internal/rules"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type errorResponse struct {
	status  int
	message string
}

type errorResponder rules.Matcher[error, errorResponse]

func newErrorResponder(ruleSet []rules.Rule[error, errorResponse], defaultStatus int, defaultMessage string) errorResponder {
	return errorResponder(rules.NewMatcher(ruleSet, func(error) errorResponse {
		return errorResponse{
			status:  defaultStatus,
			message: defaultMessage,
		}
	}))
}

func (r errorResponder) Write(c *gin.Context, err error) {
	response := rules.Matcher[error, errorResponse](r).Resolve(err)
	c.JSON(response.status, gin.H{"error": response.message})
}

func matchDomainError(target error) func(error) bool {
	return func(err error) bool {
		return errors.Is(err, target)
	}
}

func withErrorMessage(status int) func(error) errorResponse {
	return func(err error) errorResponse {
		return errorResponse{
			status:  status,
			message: err.Error(),
		}
	}
}

func withStaticMessage(status int, message string) func(error) errorResponse {
	return func(error) errorResponse {
		return errorResponse{
			status:  status,
			message: message,
		}
	}
}

var subscriptionCreateErrorResponder = newErrorResponder(
	[]rules.Rule[error, errorResponse]{
		{Match: matchDomainError(domain.ErrIncorrectRepositoryFormat), Handle: withErrorMessage(http.StatusBadRequest)},
		{Match: matchDomainError(domain.ErrAlreadyExists), Handle: withErrorMessage(http.StatusConflict)},
		{Match: matchDomainError(domain.ErrNotFound), Handle: withErrorMessage(http.StatusNotFound)},
		{Match: matchDomainError(domain.ErrRateLimited), Handle: withStaticMessage(http.StatusServiceUnavailable, "github is temporarily unavailable, please try again later")},
	},
	http.StatusInternalServerError,
	"internal server error",
)

var subscriptionConfirmErrorResponder = newErrorResponder(
	[]rules.Rule[error, errorResponse]{
		{Match: matchDomainError(domain.ErrInvalidToken), Handle: withErrorMessage(http.StatusBadRequest)},
		{Match: matchDomainError(domain.ErrNotFound), Handle: withErrorMessage(http.StatusNotFound)},
	},
	http.StatusInternalServerError,
	"internal server error",
)

var subscriptionCancelErrorResponder = newErrorResponder(
	[]rules.Rule[error, errorResponse]{
		{Match: matchDomainError(domain.ErrNotFound), Handle: withErrorMessage(http.StatusNotFound)},
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
