package mail

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github-release-notifier/internal/outbox"

	"github.com/google/uuid"
)

type renderer interface {
	RenderConfirmationEmail(data ConfirmationTemplateData) (RenderedEmail, error)
	RenderReleaseEmail(data ReleaseTemplateData) (RenderedEmail, error)
}

type outboxWriter interface {
	Create(ctx context.Context, tx *sql.Tx, recipientEmail string, email outbox.Email) error
}

type Queue interface {
	QueueSubscriptionConfirmation(ctx context.Context, tx *sql.Tx, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error
	QueueReleaseNotification(ctx context.Context, tx *sql.Tx, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error
}

type Service struct {
	renderer   renderer
	outbox     outboxWriter
	apiBaseURL string
}

func NewService(renderer renderer, outbox outboxWriter, apiBaseURL string) *Service {
	return &Service{
		renderer:   renderer,
		outbox:     outbox,
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
	}
}

func (s *Service) QueueSubscriptionConfirmation(ctx context.Context, tx *sql.Tx, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	email, err := s.renderer.RenderConfirmationEmail(ConfirmationTemplateData{
		RepositoryFullName: repositoryFullName,
		ConfirmationURL:    s.buildConfirmationURL(confirmationToken),
		CancellationURL:    s.buildCancellationURL(cancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render confirmation email: %w", err)
	}

	if err := s.outbox.Create(ctx, tx, recipientEmail, toOutboxEmail(email)); err != nil {
		return fmt.Errorf("enqueue confirmation email: %w", err)
	}

	return nil
}

func (s *Service) QueueReleaseNotification(ctx context.Context, tx *sql.Tx, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error {
	email, err := s.renderer.RenderReleaseEmail(ReleaseTemplateData{
		RepositoryFullName: repositoryFullName,
		TagName:            tagName,
		ReleaseURL:         releaseURL,
		CancellationURL:    s.buildCancellationURL(cancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render release email: %w", err)
	}

	if err := s.outbox.Create(ctx, tx, recipientEmail, toOutboxEmail(email)); err != nil {
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

func toOutboxEmail(email RenderedEmail) outbox.Email {
	return outbox.Email{
		Subject:  email.Subject,
		HTMLBody: email.HTMLBody,
	}
}
