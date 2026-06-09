package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	amqp "github.com/rabbitmq/amqp091-go"
)

type messageHandler interface {
	Handle(ctx context.Context, message notificationcontracts.Envelope) error
}

type Consumer struct {
	url         string
	exchange    string
	queue       string
	bindingKeys []string
	handler     messageHandler
}

func NewConsumer(url string, exchange string, queue string, bindingKeys []string, handler messageHandler) *Consumer {
	return &Consumer{
		url:         url,
		exchange:    exchange,
		queue:       queue,
		bindingKeys: bindingKeys,
		handler:     handler,
	}
}

func (c *Consumer) Run(ctx context.Context) {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		slog.ErrorContext(ctx, "dial rabbitmq failed", "error", err)
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	channel, err := conn.Channel()
	if err != nil {
		slog.ErrorContext(ctx, "open rabbitmq channel failed", "error", err)
		return
	}
	defer func() {
		_ = channel.Close()
	}()

	if err := channel.ExchangeDeclare(c.exchange, "topic", true, false, false, false, nil); err != nil {
		slog.ErrorContext(ctx, "declare rabbitmq exchange failed", "exchange", c.exchange, "error", err)
		return
	}

	queue, err := channel.QueueDeclare(c.queue, true, false, false, false, nil)
	if err != nil {
		slog.ErrorContext(ctx, "declare rabbitmq queue failed", "queue", c.queue, "error", err)
		return
	}

	for _, bindingKey := range c.bindingKeys {
		if err := channel.QueueBind(queue.Name, bindingKey, c.exchange, false, nil); err != nil {
			slog.ErrorContext(ctx, "bind rabbitmq queue failed", "queue", queue.Name, "binding_key", bindingKey, "error", err)
			return
		}
	}

	deliveries, err := channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		slog.ErrorContext(ctx, "consume rabbitmq queue failed", "queue", queue.Name, "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case delivery, ok := <-deliveries:
			if !ok {
				return
			}

			if err := c.handleDelivery(ctx, delivery); err != nil {
				slog.WarnContext(ctx, "handle rabbitmq delivery failed", "message_id", delivery.MessageId, "error", err)
				_ = delivery.Nack(false, true)
				continue
			}

			_ = delivery.Ack(false)
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	var message notificationcontracts.Envelope
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		return fmt.Errorf("decode notification envelope: %w", err)
	}

	return c.handler.Handle(ctx, message)
}
