//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	messagingtest "github-release-notifier/internal/platform/messaging/test"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

type integrationMessageHandlerStub struct {
	received  chan notificationcontracts.Envelope
	release   chan struct{}
	err       error
	callCount int
}

func (s *integrationMessageHandlerStub) Handle(_ context.Context, message notificationcontracts.Envelope) error {
	s.callCount++
	if s.received != nil {
		s.received <- message
	}
	if s.release != nil {
		<-s.release
	}
	return s.err
}

func TestConsumer_Run_ConsumesAndAcknowledgesMessage(t *testing.T) {
	rabbitMQURL := messagingtest.SetupRabbitMQ(t)
	_, channel := messagingtest.OpenRabbitMQChannel(t, rabbitMQURL)

	exchange := messagingtest.NewTestBrokerName("consumer-exchange")
	queue := messagingtest.NewTestBrokerName("consumer-queue")
	routingKey := "subscription.confirmation.requested"
	declareConsumerQueue(t, channel, exchange, queue, routingKey)

	handler := &integrationMessageHandlerStub{
		received: make(chan notificationcontracts.Envelope, 1),
	}
	consumer := NewConsumer(rabbitMQURL, exchange, queue, []string{routingKey}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		consumer.Run(ctx)
	}()

	expectedMessage, err := notificationcontracts.NewSubscriptionConfirmationRequestedMessage(
		uuid.New(),
		notificationcontracts.SubscriptionConfirmationRequested{
			RecipientEmail:     "user@example.com",
			RepositoryFullName: "gin-gonic/gin",
			ConfirmationToken:  uuid.New(),
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	publishEnvelope(t, channel, exchange, routingKey, expectedMessage)

	select {
	case actualMessage := <-handler.received:
		require.Equal(t, expectedMessage, actualMessage)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for consumer to handle message")
	}

	require.Eventually(t, func() bool {
		state, err := channel.QueueInspect(queue)
		require.NoError(t, err)
		return state.Messages == 0
	}, 5*time.Second, 50*time.Millisecond)

	cancel()
	<-done
}

func TestConsumer_Run_RequeuesMessageOnHandlerError(t *testing.T) {
	rabbitMQURL := messagingtest.SetupRabbitMQ(t)
	_, channel := messagingtest.OpenRabbitMQChannel(t, rabbitMQURL)

	exchange := messagingtest.NewTestBrokerName("consumer-exchange")
	queue := messagingtest.NewTestBrokerName("consumer-queue")
	routingKey := "release.notification.requested"
	declareConsumerQueue(t, channel, exchange, queue, routingKey)

	releaseHandler := make(chan struct{})
	handler := &integrationMessageHandlerStub{
		received: make(chan notificationcontracts.Envelope, 1),
		release:  releaseHandler,
		err:      errors.New("delivery failed"),
	}
	consumer := NewConsumer(rabbitMQURL, exchange, queue, []string{routingKey}, handler)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		consumer.Run(ctx)
	}()

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

	publishEnvelope(t, channel, exchange, routingKey, message)

	select {
	case <-handler.received:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for consumer to handle message")
	}

	cancel()
	close(releaseHandler)
	<-done

	require.Eventually(t, func() bool {
		state, err := channel.QueueInspect(queue)
		require.NoError(t, err)
		return state.Messages == 1
	}, 5*time.Second, 50*time.Millisecond)
	require.Equal(t, 1, handler.callCount)
}

func declareConsumerQueue(t *testing.T, channel *amqp.Channel, exchange string, queue string, routingKey string) {
	t.Helper()

	require.NoError(t, channel.ExchangeDeclare(exchange, "topic", true, false, false, false, nil))

	_, err := channel.QueueDeclare(queue, true, false, false, false, nil)
	require.NoError(t, err)

	require.NoError(t, channel.QueueBind(queue, routingKey, exchange, false, nil))
}

func publishEnvelope(t *testing.T, channel *amqp.Channel, exchange string, routingKey string, message notificationcontracts.Envelope) {
	t.Helper()

	body, err := json.Marshal(message)
	require.NoError(t, err)

	err = channel.PublishWithContext(
		context.Background(),
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			MessageId:   message.MessageID.String(),
			Type:        string(message.Type),
			Body:        body,
		},
	)
	require.NoError(t, err)
}
