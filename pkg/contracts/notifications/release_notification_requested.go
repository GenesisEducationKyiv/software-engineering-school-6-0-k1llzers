package notifications

import (
	"time"

	"github.com/google/uuid"
)

type ReleaseNotificationRequested struct {
	RecipientEmail     string    `json:"recipient_email"`
	RepositoryFullName string    `json:"repository_full_name"`
	TagName            string    `json:"tag_name"`
	ReleaseURL         string    `json:"release_url"`
	CancellationToken  uuid.UUID `json:"cancellation_token"`
}

func NewReleaseNotificationRequestedMessage(
	messageID uuid.UUID,
	occurredAt time.Time,
	payload ReleaseNotificationRequested,
) (Envelope, error) {
	return NewEnvelope(messageID, TypeReleaseNotificationRequested, occurredAt, payload)
}
