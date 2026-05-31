package mail

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/outbox"
)

const (
	defaultOutboxEmptyDelay        = time.Second
	defaultOutboxProcessingTimeout = 60
)

type outboxStore interface {
	ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (outbox.Email, error)
	MarkSent(ctx context.Context, id int64) error
	Release(ctx context.Context, id int64, lastError string) error
}

type OutboxDispatcher struct {
	store  outboxStore
	sender Sender
}

func NewOutboxDispatcher(store outboxStore, sender Sender) *OutboxDispatcher {
	return &OutboxDispatcher{
		store:  store,
		sender: sender,
	}
}

func (d *OutboxDispatcher) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		email, err := d.store.ClaimNextPending(ctx, defaultOutboxProcessingTimeout)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(defaultOutboxEmptyDelay):
					continue
				}
			}

			slog.ErrorContext(ctx, "mail outbox claim failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(defaultOutboxEmptyDelay):
				continue
			}
		}

		sendErr := d.sender.Send(ctx, email.RecipientEmail, RenderedEmail{
			Subject:  email.Subject,
			HTMLBody: email.HTMLBody,
		})
		if sendErr != nil {
			if err := d.store.Release(ctx, email.ID, sendErr.Error()); err != nil {
				slog.ErrorContext(ctx, "mail outbox release failed", "email_id", email.ID, "error", err)
			}
			slog.WarnContext(ctx, "mail send failed", "email_id", email.ID, "error", sendErr)
			continue
		}

		if err := d.store.MarkSent(ctx, email.ID); err != nil {
			slog.ErrorContext(ctx, "mail outbox mark sent failed", "email_id", email.ID, "error", err)
		}
	}
}
