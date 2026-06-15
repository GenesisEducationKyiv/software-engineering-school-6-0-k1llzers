//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	integrationoutbox "github-release-notifier/internal/app/platform/messaging/outbox"
	messagingtest "github-release-notifier/internal/platform/messaging/test"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestPublisher_Publish_SendsEnvelopeToRabbitMQ(t *testing.T) {
	rabbitMQURL := messagingtest.SetupRabbitMQ(t)
	_, channel := messagingtest.OpenRabbitMQChannel(t, rabbitMQURL)

	exchange := messagingtest.NewTestBrokerName("publisher-exchange")
	queue := messagingtest.NewTestBrokerName("publisher-queue")
	routingKey := "release.notification.requested"
	require.NoError(t, channel.ExchangeDeclare(exchange, "topic", true, false, false, false, nil))

	_, err := channel.QueueDeclare(queue, true, false, false, false, nil)
	require.NoError(t, err)
	require.NoError(t, channel.QueueBind(queue, routingKey, exchange, false, nil))

	publisher := NewPublisher(rabbitMQURL, exchange)
	messageID := uuid.New()
	payloadJSON := json.RawMessage(`{"recipient_email":"user@example.com","repository_full_name":"gin-gonic/gin"}`)

	err = publisher.Publish(context.Background(), integrationoutbox.Message{
		MessageID:   messageID,
		MessageType: routingKey,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now().UTC(),
	})
	require.NoError(t, err)

	delivery := waitForPublishedMessage(t, channel, queue)
	require.Equal(t, messageID.String(), delivery.MessageId)
	require.Equal(t, routingKey, delivery.Type)
	require.Equal(t, "application/json", delivery.ContentType)
	require.Equal(t, uint8(amqp.Persistent), delivery.DeliveryMode)

	var body struct {
		MessageID string          `json:"message_id"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(delivery.Body, &body))
	require.Equal(t, messageID.String(), body.MessageID)
	require.Equal(t, routingKey, body.Type)
	require.JSONEq(t, string(payloadJSON), string(body.Payload))
}

func waitForPublishedMessage(t *testing.T, channel *amqp.Channel, queue string) amqp.Delivery {
	t.Helper()

	var delivery amqp.Delivery
	require.Eventually(t, func() bool {
		var ok bool
		delivery, ok, _ = channel.Get(queue, true)
		return ok
	}, 5*time.Second, 50*time.Millisecond)

	return delivery
}
