package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github-release-notifier/internal/notifications"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type Sender struct {
	host string
	port int
	from string
	auth smtp.Auth
}

func NewSender(cfg Config) *Sender {
	var auth smtp.Auth
	if cfg.Username != "" || cfg.Password != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	return &Sender{
		host: cfg.Host,
		port: cfg.Port,
		from: cfg.From,
		auth: auth,
	}
}

func (s *Sender) Deliver(ctx context.Context, to string, email notifications.RenderedEmail) error {
	address := fmt.Sprintf("%s:%d", s.host, s.port)

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer func() {
		_ = client.Quit()
	}()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("start tls: %w", err)
		}
	}

	if s.auth != nil {
		if err := client.Auth(s.auth); err != nil {
			return fmt.Errorf("authenticate smtp client: %w", err)
		}
	}

	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("set mail sender: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("set mail recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open message writer: %w", err)
	}

	message := buildMessage(s.from, to, email)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write email body: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close message writer: %w", err)
	}

	return nil
}

func buildMessage(from string, to string, email notifications.RenderedEmail) string {
	headers := []string{
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"From: " + from,
		"To: " + to,
		"Subject: " + email.Subject,
	}

	return strings.Join(headers, "\r\n") + "\r\n\r\n" + email.HTMLBody
}
