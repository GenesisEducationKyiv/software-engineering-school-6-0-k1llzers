package notifications

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypeSubscriptionConfirmationRequested Type = "subscription.confirmation.requested"
	TypeReleaseNotificationRequested      Type = "release.notification.requested"
)

type Envelope struct {
	MessageID  uuid.UUID       `json:"message_id"`
	Type       Type            `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func NewEnvelope[T any](messageID uuid.UUID, messageType Type, occurredAt time.Time, payload T) (Envelope, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{
		MessageID:  messageID,
		Type:       messageType,
		OccurredAt: occurredAt.UTC(),
		Payload:    payloadJSON,
	}, nil
}
