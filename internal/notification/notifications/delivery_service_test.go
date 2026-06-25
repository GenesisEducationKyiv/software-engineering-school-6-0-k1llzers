//go:build unit

package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type rendererStub struct {
	emails map[string]RenderedEmail
	data   map[string]any
	errs   map[string]error
}

func (s *rendererStub) Render(kind string, data any) (RenderedEmail, error) {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	s.data[kind] = data

	if err := s.errs[kind]; err != nil {
		return RenderedEmail{}, err
	}

	return s.emails[kind], nil
}

type senderStub struct {
	err    error
	to     string
	email  RenderedEmail
	called bool
}

func (s *senderStub) Deliver(_ context.Context, to string, email RenderedEmail) error {
	s.called = true
	s.to = to
	s.email = email
	return s.err
}

func TestDeliveryService_DeliverSubscriptionConfirmation(t *testing.T) {
	renderer := &rendererStub{
		emails: map[string]RenderedEmail{
			templateKindConfirmation: {Subject: "Confirm subscription", HTMLBody: "<p>body</p>"},
		},
	}
	sender := &senderStub{}
	service := NewDeliveryService(renderer, sender, "http://localhost:8080/api", nil)
	confirmationToken := uuid.New()
	cancellationToken := uuid.New()
	message, err := notificationcontracts.NewSubscriptionConfirmationRequestedMessage(
		uuid.New(),
		notificationcontracts.SubscriptionConfirmationRequested{
			RecipientEmail:     "user@example.com",
			RepositoryFullName: "gin-gonic/gin",
			ConfirmationToken:  confirmationToken,
			CancellationToken:  cancellationToken,
		},
	)
	require.NoError(t, err)

	err = service.Deliver(context.Background(), message)
	require.NoError(t, err)
	require.True(t, sender.called)
	require.Equal(t, "user@example.com", sender.to)
	require.Equal(t, RenderedEmail{Subject: "Confirm subscription", HTMLBody: "<p>body</p>"}, sender.email)
}

func TestDeliveryService_DeliverReleaseNotification(t *testing.T) {
	renderer := &rendererStub{
		emails: map[string]RenderedEmail{
			templateKindRelease: {Subject: "New release", HTMLBody: "<p>release</p>"},
		},
	}
	sender := &senderStub{}
	service := NewDeliveryService(renderer, sender, "http://localhost:8080/api", nil)
	message, err := notificationcontracts.NewReleaseNotificationRequestedMessage(
		uuid.New(),
		notificationcontracts.ReleaseNotificationRequested{
			RecipientEmail:     "user@example.com",
			RepositoryFullName: "gin-gonic/gin",
			TagName:            "v1.11.0",
			ReleaseURL:         "https://example.com/release",
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	err = service.Deliver(context.Background(), message)
	require.NoError(t, err)
	require.True(t, sender.called)
	require.Equal(t, "user@example.com", sender.to)
	require.Equal(t, RenderedEmail{Subject: "New release", HTMLBody: "<p>release</p>"}, sender.email)
}

func TestDeliveryService_ReturnsDecodeError(t *testing.T) {
	service := NewDeliveryService(&rendererStub{}, &senderStub{}, "http://localhost:8080/api", nil)
	message := notificationcontracts.Envelope{
		MessageID: uuid.New(),
		Type:      notificationcontracts.TypeSubscriptionConfirmationRequested,
		Payload:   json.RawMessage(`{`),
	}

	err := service.Deliver(context.Background(), message)
	require.Error(t, err)
}

func TestDeliveryService_ReturnsSenderError(t *testing.T) {
	expectedErr := errors.New("send failed")
	renderer := &rendererStub{
		emails: map[string]RenderedEmail{
			templateKindRelease: {Subject: "New release", HTMLBody: "<p>release</p>"},
		},
	}
	service := NewDeliveryService(renderer, &senderStub{err: expectedErr}, "http://localhost:8080/api", nil)
	message, err := notificationcontracts.NewReleaseNotificationRequestedMessage(
		uuid.New(),
		notificationcontracts.ReleaseNotificationRequested{
			RecipientEmail:     "user@example.com",
			RepositoryFullName: "gin-gonic/gin",
			TagName:            "v1.11.0",
			ReleaseURL:         "https://example.com/release",
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	err = service.Deliver(context.Background(), message)
	require.ErrorIs(t, err, expectedErr)
}
