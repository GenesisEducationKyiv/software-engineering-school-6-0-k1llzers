package mail

import (
	"context"
	"fmt"

	"github-release-notifier/internal/outbox"

	"github.com/google/uuid"
)

type renderer interface {
	Render(kind string, data any) (RenderedEmail, error)
}

type outboxWriter interface {
	Create(ctx context.Context, recipientEmail string, email outbox.Email) error
}

type Service struct {
	renderer renderer
	outbox   outboxWriter
	urls     urlBuilder
}

func NewService(renderer renderer, outbox outboxWriter, apiBaseURL string) *Service {
	return &Service{
		renderer: renderer,
		outbox:   outbox,
		urls:     newURLBuilder(apiBaseURL),
	}
}

func (s *Service) QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	return s.queueTemplate(ctx, recipientEmail, templateKindConfirmation, ConfirmationTemplateData{
		RepositoryFullName: repositoryFullName,
		ConfirmationURL:    s.urls.confirmationURL(confirmationToken),
		CancellationURL:    s.urls.cancellationURL(cancellationToken),
	})
}

func (s *Service) QueueReleaseNotification(ctx context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error {
	return s.queueTemplate(ctx, recipientEmail, templateKindRelease, ReleaseTemplateData{
		RepositoryFullName: repositoryFullName,
		TagName:            tagName,
		ReleaseURL:         releaseURL,
		CancellationURL:    s.urls.cancellationURL(cancellationToken),
	})
}

func (s *Service) queueTemplate(ctx context.Context, recipientEmail string, kind string, data any) error {
	email, err := s.renderer.Render(kind, data)
	if err != nil {
		return fmt.Errorf("render %s email: %w", kind, err)
	}

	return s.enqueue(ctx, recipientEmail, email, kind)
}

func (s *Service) enqueue(ctx context.Context, recipientEmail string, email RenderedEmail, kind string) error {
	if err := s.outbox.Create(ctx, recipientEmail, toOutboxEmail(email)); err != nil {
		return fmt.Errorf("enqueue %s email: %w", kind, err)
	}

	return nil
}

func toOutboxEmail(email RenderedEmail) outbox.Email {
	return outbox.Email{
		Subject:  email.Subject,
		HTMLBody: email.HTMLBody,
	}
}
