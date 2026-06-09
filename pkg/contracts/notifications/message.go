package notifications

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Type string

const (
	TypeSubscriptionConfirmationRequested Type = "subscription.confirmation.requested"
	TypeReleaseNotificationRequested      Type = "release.notification.requested"
)

type Envelope struct {
	MessageID uuid.UUID       `json:"message_id"`
	Type      Type            `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func NewEnvelope[T any](messageID uuid.UUID, messageType Type, payload T) (Envelope, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{
		MessageID: messageID,
		Type:      messageType,
		Payload:   payloadJSON,
	}, nil
}
