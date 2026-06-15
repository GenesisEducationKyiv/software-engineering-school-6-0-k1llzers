//go:build unit

package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

type messageHandlerStub struct {
	called  bool
	message notificationcontracts.Envelope
	err     error
}

func (s *messageHandlerStub) Handle(_ context.Context, message notificationcontracts.Envelope) error {
	s.called = true
	s.message = message
	return s.err
}

func TestConsumer_HandleDelivery_DecodesEnvelopeAndCallsHandler(t *testing.T) {
	handler := &messageHandlerStub{}
	consumer := NewConsumer("", "", "", nil, handler)
	messageID := uuid.New()
	message, err := notificationcontracts.NewSubscriptionConfirmationRequestedMessage(
		messageID,
		notificationcontracts.SubscriptionConfirmationRequested{
			RecipientEmail:     "user@example.com",
			RepositoryFullName: "gin-gonic/gin",
			ConfirmationToken:  uuid.New(),
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	body, err := json.Marshal(message)
	require.NoError(t, err)

	err = consumer.handleDelivery(context.Background(), amqp.Delivery{Body: body})

	require.NoError(t, err)
	require.True(t, handler.called)
	require.Equal(t, message, handler.message)
}

func TestConsumer_HandleDelivery_ReturnsDecodeError(t *testing.T) {
	handler := &messageHandlerStub{}
	consumer := NewConsumer("", "", "", nil, handler)

	err := consumer.handleDelivery(context.Background(), amqp.Delivery{Body: []byte("{")})

	require.Error(t, err)
	require.ErrorContains(t, err, "decode notification envelope")
	require.False(t, handler.called)
}

func TestConsumer_HandleDelivery_PropagatesHandlerError(t *testing.T) {
	expectedErr := errors.New("handle failed")
	handler := &messageHandlerStub{err: expectedErr}
	consumer := NewConsumer("", "", "", nil, handler)
	message, err := notificationcontracts.NewReleaseNotificationRequestedMessage(
		uuid.New(),
		notificationcontracts.ReleaseNotificationRequested{
			RecipientEmail:     "user@example.com",
			RepositoryFullName: "gin-gonic/gin",
			TagName:            "v1.11.0",
			ReleaseURL:         "https://github.com/gin-gonic/gin/releases/tag/v1.11.0",
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	body, err := json.Marshal(message)
	require.NoError(t, err)

	err = consumer.handleDelivery(context.Background(), amqp.Delivery{Body: body})

	require.ErrorIs(t, err, expectedErr)
	require.True(t, handler.called)
}
