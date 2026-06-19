package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	integrationoutbox "github-release-notifier/internal/app/platform/messaging/outbox"

	amqp "github.com/rabbitmq/amqp091-go"
)

var errPublishNotAcknowledged = errors.New("rabbitmq publish was not acknowledged")

type Publisher struct {
	url      string
	exchange string
}

func NewPublisher(url string, exchange string) *Publisher {
	return &Publisher{
		url:      url,
		exchange: exchange,
	}
}

type envelope struct {
	MessageID string          `json:"message_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func (p *Publisher) Publish(ctx context.Context, message integrationoutbox.Message) error {
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer func() {
		_ = channel.Close()
	}()

	if err := channel.ExchangeDeclare(
		p.exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare rabbitmq exchange: %w", err)
	}

	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable rabbitmq publisher confirms: %w", err)
	}

	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	body, err := json.Marshal(envelope{
		MessageID: message.MessageID.String(),
		Type:      message.MessageType,
		Payload:   message.PayloadJSON,
	})
	if err != nil {
		return fmt.Errorf("marshal integration message envelope: %w", err)
	}

	if err := channel.PublishWithContext(
		ctx,
		p.exchange,
		message.MessageType,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    message.MessageID.String(),
			Type:         message.MessageType,
			Timestamp:    message.CreatedAt,
			Body:         body,
		},
	); err != nil {
		return fmt.Errorf("publish to rabbitmq: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case confirmation, ok := <-confirmations:
		if !ok {
			return errPublishNotAcknowledged
		}
		if !confirmation.Ack {
			return errPublishNotAcknowledged
		}
	}

	return nil
}
