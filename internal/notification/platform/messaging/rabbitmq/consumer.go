package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultReconnectDelay = 5 * time.Second

type messageHandler interface {
	Handle(ctx context.Context, message notificationcontracts.Envelope) error
}

type Consumer struct {
	url            string
	exchange       string
	queue          string
	bindingKeys    []string
	handler        messageHandler
	reconnectDelay time.Duration
}

func NewConsumer(url string, exchange string, queue string, bindingKeys []string, handler messageHandler) *Consumer {
	return &Consumer{
		url:            url,
		exchange:       exchange,
		queue:          queue,
		bindingKeys:    bindingKeys,
		handler:        handler,
		reconnectDelay: defaultReconnectDelay,
	}
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := c.runOnce(ctx); err != nil {
			slog.WarnContext(ctx, "rabbitmq consumer stopped, reconnecting", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(c.reconnectDelay):
		}
	}
}

func (c *Consumer) runOnce(ctx context.Context) error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()
	connectionClosed := conn.NotifyClose(make(chan *amqp.Error, 1))

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer func() {
		_ = channel.Close()
	}()
	channelClosed := channel.NotifyClose(make(chan *amqp.Error, 1))

	if err := channel.ExchangeDeclare(c.exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare rabbitmq exchange %q: %w", c.exchange, err)
	}

	queue, err := channel.QueueDeclare(c.queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare rabbitmq queue %q: %w", c.queue, err)
	}

	for _, bindingKey := range c.bindingKeys {
		if err := channel.QueueBind(queue.Name, bindingKey, c.exchange, false, nil); err != nil {
			return fmt.Errorf("bind rabbitmq queue %q with routing key %q: %w", queue.Name, bindingKey, err)
		}
	}

	deliveries, err := channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume rabbitmq queue %q: %w", queue.Name, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-connectionClosed:
			return formatAMQPCloseError("rabbitmq connection closed", err, ok)
		case err, ok := <-channelClosed:
			return formatAMQPCloseError("rabbitmq channel closed", err, ok)
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq deliveries channel closed")
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

func formatAMQPCloseError(message string, err *amqp.Error, ok bool) error {
	if !ok || err == nil {
		return errors.New(message)
	}

	return fmt.Errorf("%s: %w", message, err)
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	var message notificationcontracts.Envelope
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		return fmt.Errorf("decode notification envelope: %w", err)
	}

	return c.handler.Handle(ctx, message)
}
