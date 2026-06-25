package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	notificationmetrics "github-release-notifier/internal/notification/platform/metrics"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"
)

type renderer interface {
	Render(kind string, data any) (RenderedEmail, error)
}

type emailDeliveryGateway interface {
	Deliver(ctx context.Context, to string, email RenderedEmail) error
}

type DeliveryService struct {
	renderer renderer
	sender   emailDeliveryGateway
	urls     urlBuilder
	metrics  *notificationmetrics.Metrics
}

func NewDeliveryService(renderer renderer, sender emailDeliveryGateway, apiBaseURL string, metricSet *notificationmetrics.Metrics) *DeliveryService {
	return &DeliveryService{
		renderer: renderer,
		sender:   sender,
		urls:     newURLBuilder(apiBaseURL),
		metrics:  metricSet,
	}
}

func (s *DeliveryService) Deliver(ctx context.Context, message notificationcontracts.Envelope) (err error) {
	startedAt := time.Now()
	defer func() {
		if s.metrics == nil {
			return
		}

		s.metrics.ObserveEmailDelivery(ctx, string(message.Type), err == nil, time.Since(startedAt))
	}()

	switch message.Type {
	case notificationcontracts.TypeSubscriptionConfirmationRequested:
		return s.deliverSubscriptionConfirmation(ctx, message.Payload)
	case notificationcontracts.TypeReleaseNotificationRequested:
		return s.deliverReleaseNotification(ctx, message.Payload)
	default:
		return fmt.Errorf("unsupported notification message type: %s", message.Type)
	}
}

func (s *DeliveryService) deliverSubscriptionConfirmation(ctx context.Context, payloadJSON json.RawMessage) error {
	var payload notificationcontracts.SubscriptionConfirmationRequested
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return fmt.Errorf("decode subscription confirmation notification payload: %w", err)
	}

	email, err := s.renderer.Render(templateKindConfirmation, ConfirmationTemplateData{
		RepositoryFullName: payload.RepositoryFullName,
		ConfirmationURL:    s.urls.confirmationURL(payload.ConfirmationToken),
		CancellationURL:    s.urls.cancellationURL(payload.CancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render confirmation email: %w", err)
	}

	if err := s.sender.Deliver(ctx, payload.RecipientEmail, email); err != nil {
		return fmt.Errorf("deliver confirmation email: %w", err)
	}

	return nil
}

func (s *DeliveryService) deliverReleaseNotification(ctx context.Context, payloadJSON json.RawMessage) error {
	var payload notificationcontracts.ReleaseNotificationRequested
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return fmt.Errorf("decode release notification payload: %w", err)
	}

	email, err := s.renderer.Render(templateKindRelease, ReleaseTemplateData{
		RepositoryFullName: payload.RepositoryFullName,
		TagName:            payload.TagName,
		ReleaseURL:         payload.ReleaseURL,
		CancellationURL:    s.urls.cancellationURL(payload.CancellationToken),
	})
	if err != nil {
		return fmt.Errorf("render release email: %w", err)
	}

	if err := s.sender.Deliver(ctx, payload.RecipientEmail, email); err != nil {
		return fmt.Errorf("deliver release email: %w", err)
	}

	return nil
}
