package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type confirmationRenderer interface {
	RenderConfirmationEmail(data ConfirmationTemplateData) (RenderedEmail, error)
}

type Service struct {
	renderer   confirmationRenderer
	sender     Sender
	apiBaseUrl string
}

func NewService(renderer confirmationRenderer, sender Sender, apiBaseUrl string) *Service {
	return &Service{
		renderer:   renderer,
		sender:     sender,
		apiBaseUrl: strings.TrimRight(apiBaseUrl, "/"),
	}
}

func (s *Service) SendSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	email, err := s.renderer.RenderConfirmationEmail(ConfirmationTemplateData{
		RepositoryFullName: repositoryFullName,
		ConfirmationURL:    s.buildConfirmationURL(confirmationToken),
		CancellationURL:    s.buildCancellationURL(cancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render confirmation email: %w", err)
	}

	if err := s.sender.Send(ctx, recipientEmail, email); err != nil {
		return fmt.Errorf("send confirmation email: %w", err)
	}

	return nil
}

func (s *Service) buildConfirmationURL(confirmationToken uuid.UUID) string {
	return s.apiBaseUrl + "/confirm/" + confirmationToken.String()
}

func (s *Service) buildCancellationURL(cancellationToken uuid.UUID) string {
	return s.apiBaseUrl + "/unsubscribe/" + cancellationToken.String()
}
