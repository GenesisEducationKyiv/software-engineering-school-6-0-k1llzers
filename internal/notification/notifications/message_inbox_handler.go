package notifications

import (
	"context"
	"encoding/json"
	"fmt"

	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
)

type messageInboxStore interface {
	ClaimForProcessing(ctx context.Context, messageID uuid.UUID, messageType string, payloadJSON json.RawMessage) (bool, error)
	MarkProcessed(ctx context.Context, messageID uuid.UUID) error
	ReleaseProcessing(ctx context.Context, messageID uuid.UUID) error
}

type messageDelivery interface {
	Deliver(ctx context.Context, message notificationcontracts.Envelope) error
}
type MessageInboxHandler struct {
	inbox    messageInboxStore
	delivery messageDelivery
}

func NewMessageInboxHandler(inbox messageInboxStore, delivery messageDelivery) *MessageInboxHandler {
	return &MessageInboxHandler{
		inbox:    inbox,
		delivery: delivery,
	}
}

func (h *MessageInboxHandler) Handle(ctx context.Context, message notificationcontracts.Envelope) error {
	if !isSupportedNotificationMessageType(message.Type) {
		return fmt.Errorf("unsupported notification message type: %s", message.Type)
	}

	claimed, err := h.inbox.ClaimForProcessing(ctx, message.MessageID, string(message.Type), message.Payload)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	if err := h.delivery.Deliver(ctx, message); err != nil {
		releaseErr := h.inbox.ReleaseProcessing(ctx, message.MessageID)
		if releaseErr != nil {
			return fmt.Errorf("release notification message processing after failed delivery: %v: %w", releaseErr, err)
		}

		return err
	}

	return h.inbox.MarkProcessed(ctx, message.MessageID)
}

func isSupportedNotificationMessageType(messageType notificationcontracts.Type) bool {
	switch messageType {
	case notificationcontracts.TypeSubscriptionConfirmationRequested, notificationcontracts.TypeReleaseNotificationRequested:
		return true
	default:
		return false
	}
}
