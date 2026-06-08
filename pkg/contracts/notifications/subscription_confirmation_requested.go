package notifications

import (
	"time"

	"github.com/google/uuid"
)

type SubscriptionConfirmationRequested struct {
	RecipientEmail     string    `json:"recipient_email"`
	RepositoryFullName string    `json:"repository_full_name"`
	ConfirmationToken  uuid.UUID `json:"confirmation_token"`
	CancellationToken  uuid.UUID `json:"cancellation_token"`
}

func NewSubscriptionConfirmationRequestedMessage(
	messageID uuid.UUID,
	occurredAt time.Time,
	payload SubscriptionConfirmationRequested,
) (Envelope, error) {
	return NewEnvelope(messageID, TypeSubscriptionConfirmationRequested, occurredAt, payload)
}
