//go:build unit

package smtp

import (
	"testing"

	"github-release-notifier/internal/notification/notifications"

	"github.com/stretchr/testify/require"
)

func TestNewSMTPSender_WithCredentials_CreatesAuth(t *testing.T) {
	sender := NewSender(Config{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user",
		Password: "pass",
		From:     "noreply@example.com",
	})

	require.Equal(t, "smtp.example.com", sender.host)
	require.Equal(t, 587, sender.port)
	require.Equal(t, "noreply@example.com", sender.from)
	require.NotNil(t, sender.auth)
}

func TestNewSMTPSender_WithoutCredentials_DoesNotCreateAuth(t *testing.T) {
	sender := NewSender(Config{
		Host: "smtp.example.com",
		Port: 25,
		From: "noreply@example.com",
	})

	require.Nil(t, sender.auth)
}

func TestBuildMessage(t *testing.T) {
	message := buildMessage("from@example.com", "to@example.com", notifications.RenderedEmail{
		Subject:  "subject",
		HTMLBody: "<p>body</p>",
	})

	require.Contains(t, message, "MIME-Version: 1.0")
	require.Contains(t, message, "Content-Type: text/html; charset=UTF-8")
	require.Contains(t, message, "From: from@example.com")
	require.Contains(t, message, "To: to@example.com")
	require.Contains(t, message, "Subject: subject")
	require.Contains(t, message, "\r\n\r\n<p>body</p>")
}
