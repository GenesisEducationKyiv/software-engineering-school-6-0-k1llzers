//go:build unit

package notifications

import (
	"context"
	"errors"
	"testing"

	"github-release-notifier/internal/outbox"

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

type outboxWriterStub struct {
	err            error
	recipientEmail string
	email          outbox.Email
	called         bool
}

func (s *outboxWriterStub) Create(_ context.Context, recipientEmail string, email outbox.Email) error {
	s.called = true
	s.recipientEmail = recipientEmail
	s.email = email
	return s.err
}

func TestService_QueueSubscriptionConfirmation(t *testing.T) {
	renderer := &rendererStub{
		emails: map[string]RenderedEmail{
			templateKindConfirmation: {
				Subject:  "Confirm subscription",
				HTMLBody: "<p>body</p>",
			},
		},
	}
	outboxStore := &outboxWriterStub{}
	service := NewService(renderer, outboxStore, "http://localhost:8080/api/")

	err := service.QueueSubscriptionConfirmation(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	)

	require.NoError(t, err)
	require.True(t, outboxStore.called)
	require.Equal(t, "user@example.com", outboxStore.recipientEmail)
	require.Equal(t, outbox.Email{Subject: "Confirm subscription", HTMLBody: "<p>body</p>"}, outboxStore.email)
	confirmationData := renderer.data[templateKindConfirmation].(ConfirmationTemplateData)
	require.Equal(t, "gin-gonic/gin", confirmationData.RepositoryFullName)
	require.Equal(t, "http://localhost:8080/api/confirm/11111111-1111-1111-1111-111111111111", confirmationData.ConfirmationURL)
	require.Equal(t, "http://localhost:8080/api/unsubscribe/22222222-2222-2222-2222-222222222222", confirmationData.CancellationURL)
}

func TestService_QueueSubscriptionConfirmation_ReturnsRendererError(t *testing.T) {
	expectedErr := errors.New("render failed")
	service := NewService(&rendererStub{errs: map[string]error{templateKindConfirmation: expectedErr}}, &outboxWriterStub{}, "http://localhost:8080/api")

	err := service.QueueSubscriptionConfirmation(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		uuid.New(),
		uuid.New(),
	)

	require.ErrorIs(t, err, expectedErr)
	require.Contains(t, err.Error(), "render confirmation email")
}

func TestService_QueueSubscriptionConfirmation_ReturnsOutboxError(t *testing.T) {
	expectedErr := errors.New("enqueue failed")
	service := NewService(
		&rendererStub{emails: map[string]RenderedEmail{templateKindConfirmation: {Subject: "subject", HTMLBody: "body"}}},
		&outboxWriterStub{err: expectedErr},
		"http://localhost:8080/api",
	)

	err := service.QueueSubscriptionConfirmation(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		uuid.New(),
		uuid.New(),
	)

	require.ErrorIs(t, err, expectedErr)
	require.Contains(t, err.Error(), "enqueue confirmation email")
}

func TestService_QueueReleaseNotification(t *testing.T) {
	renderer := &rendererStub{
		emails: map[string]RenderedEmail{
			templateKindRelease: {
				Subject:  "New release",
				HTMLBody: "<p>release</p>",
			},
		},
	}
	outboxStore := &outboxWriterStub{}
	service := NewService(renderer, outboxStore, "http://localhost:8080/api")

	err := service.QueueReleaseNotification(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		"v1.11.0",
		"https://github.com/gin-gonic/gin/releases/tag/v1.11.0",
		uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	)

	require.NoError(t, err)
	require.True(t, outboxStore.called)
	require.Equal(t, outbox.Email{Subject: "New release", HTMLBody: "<p>release</p>"}, outboxStore.email)
	releaseData := renderer.data[templateKindRelease].(ReleaseTemplateData)
	require.Equal(t, "gin-gonic/gin", releaseData.RepositoryFullName)
	require.Equal(t, "v1.11.0", releaseData.TagName)
	require.Equal(t, "https://github.com/gin-gonic/gin/releases/tag/v1.11.0", releaseData.ReleaseURL)
	require.Equal(t, "http://localhost:8080/api/unsubscribe/22222222-2222-2222-2222-222222222222", releaseData.CancellationURL)
}

func TestService_QueueReleaseNotification_ReturnsRendererError(t *testing.T) {
	expectedErr := errors.New("render failed")
	service := NewService(&rendererStub{errs: map[string]error{templateKindRelease: expectedErr}}, &outboxWriterStub{}, "http://localhost:8080/api")

	err := service.QueueReleaseNotification(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		"v1.11.0",
		"https://example.com/release",
		uuid.New(),
	)

	require.ErrorIs(t, err, expectedErr)
	require.Contains(t, err.Error(), "render release email")
}

func TestService_QueueReleaseNotification_ReturnsOutboxError(t *testing.T) {
	expectedErr := errors.New("enqueue failed")
	service := NewService(
		&rendererStub{emails: map[string]RenderedEmail{templateKindRelease: {Subject: "subject", HTMLBody: "body"}}},
		&outboxWriterStub{err: expectedErr},
		"http://localhost:8080/api",
	)

	err := service.QueueReleaseNotification(
		context.Background(),
		"user@example.com",
		"gin-gonic/gin",
		"v1.11.0",
		"https://example.com/release",
		uuid.New(),
	)

	require.ErrorIs(t, err, expectedErr)
	require.Contains(t, err.Error(), "enqueue release email")
}
