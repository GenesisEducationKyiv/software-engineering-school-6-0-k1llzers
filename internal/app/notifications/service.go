package notifications

import (
	"context"
	"fmt"

	integrationoutbox "github-release-notifier/internal/app/platform/messaging/outbox"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
)

type outboxWriter interface {
	Create(ctx context.Context, message integrationoutbox.Message) error
}

type Service struct {
	outbox outboxWriter
}

func NewService(outbox outboxWriter) *Service {
	return &Service{outbox: outbox}
}

func (s *Service) QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	message, err := notificationcontracts.NewSubscriptionConfirmationRequestedMessage(
		uuid.New(),
		notificationcontracts.SubscriptionConfirmationRequested{
			RecipientEmail:     recipientEmail,
			RepositoryFullName: repositoryFullName,
			ConfirmationToken:  confirmationToken,
			CancellationToken:  cancellationToken,
		},
	)
	if err != nil {
		return fmt.Errorf("build subscription confirmation notification message: %w", err)
	}

	return s.outbox.Create(ctx, integrationoutbox.Message{
		MessageID:   message.MessageID,
		MessageType: string(message.Type),
		PayloadJSON: message.Payload,
	})
}

func (s *Service) QueueReleaseNotification(ctx context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error {
	message, err := notificationcontracts.NewReleaseNotificationRequestedMessage(
		uuid.New(),
		notificationcontracts.ReleaseNotificationRequested{
			RecipientEmail:     recipientEmail,
			RepositoryFullName: repositoryFullName,
			TagName:            tagName,
			ReleaseURL:         releaseURL,
			CancellationToken:  cancellationToken,
		},
	)
	if err != nil {
		return fmt.Errorf("build release notification message: %w", err)
	}

	return s.outbox.Create(ctx, integrationoutbox.Message{
		MessageID:   message.MessageID,
		MessageType: string(message.Type),
		PayloadJSON: message.Payload,
	})
}
