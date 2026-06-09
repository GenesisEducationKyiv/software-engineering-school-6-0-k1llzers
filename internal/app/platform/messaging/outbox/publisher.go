package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github-release-notifier/internal/shared"
)

const (
	defaultOutboxPublisherEmptyDelay        = time.Second
	defaultOutboxPublisherProcessingTimeout = 60
)

type claimStore interface {
	ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (Message, error)
	MarkPublished(ctx context.Context, id int64) error
	Release(ctx context.Context, id int64) error
}

type messagePublisher interface {
	Publish(ctx context.Context, message Message) error
}

type PublisherWorker struct {
	store     claimStore
	publisher messagePublisher
}

func NewPublisherWorker(store claimStore, publisher messagePublisher) *PublisherWorker {
	return &PublisherWorker{
		store:     store,
		publisher: publisher,
	}
}

func (w *PublisherWorker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		message, err := w.store.ClaimNextPending(ctx, defaultOutboxPublisherProcessingTimeout)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(defaultOutboxPublisherEmptyDelay):
					continue
				}
			}

			slog.ErrorContext(ctx, "integration outbox claim failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(defaultOutboxPublisherEmptyDelay):
				continue
			}
		}

		if err := w.publisher.Publish(ctx, message); err != nil {
			if releaseErr := w.store.Release(ctx, message.ID); releaseErr != nil {
				slog.ErrorContext(ctx, "integration outbox release failed", "message_id", message.MessageID.String(), "error", releaseErr)
				continue
			}

			slog.WarnContext(ctx, "integration outbox publish failed", "message_id", message.MessageID.String(), "error", err)
			continue
		}

		if err := w.store.MarkPublished(ctx, message.ID); err != nil {
			slog.ErrorContext(ctx, "integration outbox mark published failed", "message_id", message.MessageID.String(), "error", err)
		}
	}
}
