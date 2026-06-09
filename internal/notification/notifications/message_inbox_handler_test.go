//go:build unit

package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type messageInboxStoreStub struct {
	claimCalled   bool
	markCalled    bool
	releaseCalled bool
	claimed       bool
	messageID     uuid.UUID
	messageType   string
	payloadJSON   json.RawMessage
	err           error
}

func (s *messageInboxStoreStub) ClaimForProcessing(_ context.Context, messageID uuid.UUID, messageType string, payloadJSON json.RawMessage) (bool, error) {
	s.claimCalled = true
	s.messageID = messageID
	s.messageType = messageType
	s.payloadJSON = payloadJSON
	return s.claimed, s.err
}

func (s *messageInboxStoreStub) MarkProcessed(_ context.Context, messageID uuid.UUID) error {
	s.markCalled = true
	s.messageID = messageID
	return s.err
}

func (s *messageInboxStoreStub) ReleaseProcessing(_ context.Context, messageID uuid.UUID) error {
	s.releaseCalled = true
	s.messageID = messageID
	return s.err
}

type messageDeliveryStub struct {
	called  bool
	message notificationcontracts.Envelope
	err     error
}

func (s *messageDeliveryStub) Deliver(_ context.Context, message notificationcontracts.Envelope) error {
	s.called = true
	s.message = message
	return s.err
}

func TestMessageInboxHandler_Handle_ClaimsDeliversAndMarksProcessed(t *testing.T) {
	inbox := &messageInboxStoreStub{claimed: true}
	delivery := &messageDeliveryStub{}
	handler := NewMessageInboxHandler(inbox, delivery)
	messageID := uuid.New()
	message, err := notificationcontracts.NewEnvelope(
		messageID,
		notificationcontracts.TypeSubscriptionConfirmationRequested,
		map[string]string{"recipient_email": "test@example.com"},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.NoError(t, err)
	require.True(t, inbox.claimCalled)
	require.True(t, delivery.called)
	require.True(t, inbox.markCalled)
	require.False(t, inbox.releaseCalled)
	require.Equal(t, messageID, inbox.messageID)
	require.Equal(t, string(notificationcontracts.TypeSubscriptionConfirmationRequested), inbox.messageType)
}

func TestMessageInboxHandler_Handle_ReturnsUnsupportedTypeError(t *testing.T) {
	inbox := &messageInboxStoreStub{claimed: true}
	handler := NewMessageInboxHandler(inbox, &messageDeliveryStub{})
	messageID := uuid.New()
	message, err := notificationcontracts.NewEnvelope(
		messageID,
		notificationcontracts.Type("unknown.message"),
		map[string]string{"value": "test"},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.Error(t, err)
	require.False(t, inbox.claimCalled)
}

func TestMessageInboxHandler_Handle_ReturnsClaimError(t *testing.T) {
	expectedErr := errors.New("claim failed")
	inbox := &messageInboxStoreStub{claimed: false, err: expectedErr}
	handler := NewMessageInboxHandler(inbox, &messageDeliveryStub{})
	messageID := uuid.New()
	message, err := notificationcontracts.NewEnvelope(
		messageID,
		notificationcontracts.TypeReleaseNotificationRequested,
		map[string]string{"recipient_email": "test@example.com"},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.ErrorIs(t, err, expectedErr)
}

func TestMessageInboxHandler_Handle_SkipsDuplicateMessage(t *testing.T) {
	inbox := &messageInboxStoreStub{claimed: false}
	delivery := &messageDeliveryStub{}
	handler := NewMessageInboxHandler(inbox, delivery)
	messageID := uuid.New()
	message, err := notificationcontracts.NewEnvelope(
		messageID,
		notificationcontracts.TypeReleaseNotificationRequested,
		map[string]string{"recipient_email": "test@example.com"},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.NoError(t, err)
	require.True(t, inbox.claimCalled)
	require.False(t, delivery.called)
	require.False(t, inbox.markCalled)
	require.False(t, inbox.releaseCalled)
}

func TestMessageInboxHandler_Handle_ReleasesProcessingOnDeliveryError(t *testing.T) {
	expectedErr := errors.New("deliver failed")
	inbox := &messageInboxStoreStub{claimed: true}
	delivery := &messageDeliveryStub{err: expectedErr}
	handler := NewMessageInboxHandler(inbox, delivery)
	messageID := uuid.New()
	message, err := notificationcontracts.NewEnvelope(
		messageID,
		notificationcontracts.TypeReleaseNotificationRequested,
		map[string]string{"recipient_email": "test@example.com"},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.ErrorIs(t, err, expectedErr)
	require.True(t, inbox.releaseCalled)
	require.False(t, inbox.markCalled)
}
