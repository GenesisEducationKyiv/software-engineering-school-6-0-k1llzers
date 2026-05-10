package mail

import (
	"strings"

	"github.com/google/uuid"
)

type urlBuilder struct {
	apiBaseURL string
}

func newURLBuilder(apiBaseURL string) urlBuilder {
	return urlBuilder{apiBaseURL: strings.TrimRight(apiBaseURL, "/")}
}

func (b urlBuilder) confirmationURL(confirmationToken uuid.UUID) string {
	return b.apiBaseURL + "/confirm/" + confirmationToken.String()
}

func (b urlBuilder) cancellationURL(cancellationToken uuid.UUID) string {
	return b.apiBaseURL + "/unsubscribe/" + cancellationToken.String()
}
