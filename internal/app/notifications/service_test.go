//go:build unit

package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	integrationoutbox "github-release-notifier/internal/app/platform/messaging/outbox"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type outboxWriterStub struct {
	err     error
	message integrationoutbox.Message
	called  bool
}

func (s *outboxWriterStub) Create(_ context.Context, message integrationoutbox.Message) error {
	s.called = true
	s.message = message
	return s.err
}

func TestService_QueueSubscriptionConfirmation(t *testing.T) {
	outboxStore := &outboxWriterStub{}
	service := NewService(outboxStore)
	confirmationToken := uuid.New()
	cancellationToken := uuid.New()

	err := service.QueueSubscriptionConfirmation(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		confirmationToken,
		cancellationToken,
	)

	require.NoError(t, err)
	require.True(t, outboxStore.called)
	require.NotEqual(t, uuid.Nil, outboxStore.message.MessageID)
	require.Equal(t, string(notificationcontracts.TypeSubscriptionConfirmationRequested), outboxStore.message.MessageType)

	var payload notificationcontracts.SubscriptionConfirmationRequested
	require.NoError(t, json.Unmarshal(outboxStore.message.PayloadJSON, &payload))
	require.Equal(t, "user@example.com", payload.RecipientEmail)
	require.Equal(t, "gin-gonic/gin", payload.RepositoryFullName)
	require.Equal(t, confirmationToken, payload.ConfirmationToken)
	require.Equal(t, cancellationToken, payload.CancellationToken)
}

func TestService_QueueSubscriptionConfirmation_ReturnsOutboxError(t *testing.T) {
	expectedErr := errors.New("enqueue failed")
	service := NewService(&outboxWriterStub{err: expectedErr})

	err := service.QueueSubscriptionConfirmation(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		uuid.New(),
		uuid.New(),
	)

	require.ErrorIs(t, err, expectedErr)
}

func TestService_QueueReleaseNotification(t *testing.T) {
	outboxStore := &outboxWriterStub{}
	service := NewService(outboxStore)
	cancellationToken := uuid.New()

	err := service.QueueReleaseNotification(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		"v1.11.0",
		"https://github.com/gin-gonic/gin/releases/tag/v1.11.0",
		cancellationToken,
	)

	require.NoError(t, err)
	require.True(t, outboxStore.called)
	require.NotEqual(t, uuid.Nil, outboxStore.message.MessageID)
	require.Equal(t, string(notificationcontracts.TypeReleaseNotificationRequested), outboxStore.message.MessageType)

	var payload notificationcontracts.ReleaseNotificationRequested
	require.NoError(t, json.Unmarshal(outboxStore.message.PayloadJSON, &payload))
	require.Equal(t, "user@example.com", payload.RecipientEmail)
	require.Equal(t, "gin-gonic/gin", payload.RepositoryFullName)
	require.Equal(t, "v1.11.0", payload.TagName)
	require.Equal(t, "https://github.com/gin-gonic/gin/releases/tag/v1.11.0", payload.ReleaseURL)
	require.Equal(t, cancellationToken, payload.CancellationToken)
}

func TestService_QueueReleaseNotification_ReturnsOutboxError(t *testing.T) {
	expectedErr := errors.New("enqueue failed")
	service := NewService(&outboxWriterStub{err: expectedErr})

	err := service.QueueReleaseNotification(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		"v1.11.0",
		"https://example.com/release",
		uuid.New(),
	)

	require.ErrorIs(t, err, expectedErr)
}
