package mail

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github-release-notifier/internal/domain"
	appmetrics "github-release-notifier/internal/metrics"
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
	store   outboxStore
	sender  Sender
	metrics *appmetrics.Metrics
}

func NewOutboxDispatcher(store outboxStore, sender Sender, metricSet *appmetrics.Metrics) *OutboxDispatcher {
	if metricSet == nil {
		panic("metrics is required")
	}

	return &OutboxDispatcher{
		store:   store,
		sender:  sender,
		metrics: metricSet,
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

		processingStartedAt := time.Now()
		sendErr := d.sender.Send(ctx, email.RecipientEmail, RenderedEmail{
			Subject:  email.Subject,
			HTMLBody: email.HTMLBody,
		})
		dispatchDuration := time.Since(processingStartedAt)
		if sendErr != nil {
			if err := d.store.Release(ctx, email.ID, sendErr.Error()); err != nil {
				d.metrics.ObserveOutboxDispatch(ctx, "release_failed", dispatchDuration)
				slog.ErrorContext(ctx, "mail outbox release failed", "email_id", email.ID, "error", err)
				continue
			}
			d.metrics.ObserveOutboxDispatch(ctx, "send_failed", dispatchDuration)
			slog.WarnContext(ctx, "mail send failed", "email_id", email.ID, "error", sendErr)
			continue
		}

		if err := d.store.MarkSent(ctx, email.ID); err != nil {
			d.metrics.ObserveOutboxDispatch(ctx, "mark_sent_failed", dispatchDuration)
			slog.ErrorContext(ctx, "mail outbox mark sent failed", "email_id", email.ID, "error", err)
			continue
		}

		d.metrics.ObserveOutboxDispatch(ctx, "success", dispatchDuration)
	}
}
