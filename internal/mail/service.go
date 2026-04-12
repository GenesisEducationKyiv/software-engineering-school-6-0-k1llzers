package mail

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/storage"

	"github.com/google/uuid"
)

type renderer interface {
	RenderConfirmationEmail(data ConfirmationTemplateData) (RenderedEmail, error)
	RenderReleaseEmail(data ReleaseTemplateData) (RenderedEmail, error)
}

type Queue interface {
	QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error
	QueueReleaseNotification(ctx context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error
}

type Service struct {
	renderer   renderer
	outbox     *storage.OutboxStore
	apiBaseURL string
}

func NewService(renderer renderer, outbox *storage.OutboxStore, apiBaseURL string) *Service {
	return &Service{
		renderer:   renderer,
		outbox:     outbox,
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
	}
}

func (s *Service) WithTx(tx *sql.Tx) Queue {
	return &Service{
		renderer:   s.renderer,
		outbox:     s.outbox.WithTx(tx),
		apiBaseURL: s.apiBaseURL,
	}
}

func (s *Service) QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	email, err := s.renderer.RenderConfirmationEmail(ConfirmationTemplateData{
		RepositoryFullName: repositoryFullName,
		ConfirmationURL:    s.buildConfirmationURL(confirmationToken),
		CancellationURL:    s.buildCancellationURL(cancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render confirmation email: %w", err)
	}

	if err := s.outbox.Create(ctx, recipientEmail, toOutboxEmail(email)); err != nil {
		return fmt.Errorf("enqueue confirmation email: %w", err)
	}

	return nil
}

func (s *Service) QueueReleaseNotification(ctx context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error {
	email, err := s.renderer.RenderReleaseEmail(ReleaseTemplateData{
		RepositoryFullName: repositoryFullName,
		TagName:            tagName,
		ReleaseURL:         releaseURL,
		CancellationURL:    s.buildCancellationURL(cancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render release email: %w", err)
	}

	if err := s.outbox.Create(ctx, recipientEmail, toOutboxEmail(email)); err != nil {
		return fmt.Errorf("enqueue release email: %w", err)
	}

	return nil
}

func (s *Service) buildConfirmationURL(confirmationToken uuid.UUID) string {
	return s.apiBaseURL + "/confirm/" + confirmationToken.String()
}

func (s *Service) buildCancellationURL(cancellationToken uuid.UUID) string {
	return s.apiBaseURL + "/unsubscribe/" + cancellationToken.String()
}

func toOutboxEmail(email RenderedEmail) domain.OutboxEmail {
	return domain.OutboxEmail{
		Subject:  email.Subject,
		HTMLBody: email.HTMLBody,
		TextBody: email.TextBody,
	}
}
