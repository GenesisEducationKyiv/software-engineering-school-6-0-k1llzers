package mail

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateRenderer_RenderConfirmationEmail(t *testing.T) {
	renderer, err := NewTemplateRenderer()
	require.NoError(t, err)

	email, err := renderer.RenderConfirmationEmail(ConfirmationTemplateData{
		RepositoryFullName: "gin-gonic/gin",
		ConfirmationURL:    "http://localhost:8080/confirm/abc",
		CancellationURL:    "http://localhost:8080/unsubscribe/def",
	})
	require.NoError(t, err)
	require.Equal(t, confirmationEmailSubject, email.Subject)
	require.Contains(t, email.HTMLBody, "To start receiving notifications about new releases")
	require.Contains(t, email.HTMLBody, "gin-gonic/gin")
	require.Contains(t, email.HTMLBody, "http://localhost:8080/confirm/abc")
	require.Contains(t, email.HTMLBody, "http://localhost:8080/unsubscribe/def")
}

func TestTemplateRenderer_RenderReleaseEmail(t *testing.T) {
	renderer, err := NewTemplateRenderer()
	require.NoError(t, err)

	email, err := renderer.RenderReleaseEmail(ReleaseTemplateData{
		RepositoryFullName: "gin-gonic/gin",
		TagName:            "v1.11.0",
		ReleaseURL:         "https://github.com/gin-gonic/gin/releases/tag/v1.11.0",
		CancellationURL:    "http://localhost:8080/unsubscribe/def",
	})
	require.NoError(t, err)
	require.Equal(t, "New release for gin-gonic/gin: v1.11.0", email.Subject)
	require.Contains(t, email.HTMLBody, "New release available")
	require.Contains(t, email.HTMLBody, "https://github.com/gin-gonic/gin/releases/tag/v1.11.0")
}
