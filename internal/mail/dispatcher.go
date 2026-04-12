package mail

import (
	"context"
	"errors"
	"log"
	"time"

	"github-release-notifier/internal/domain"
)

const (
	defaultOutboxEmptyDelay        = time.Second
	defaultOutboxProcessingTimeout = 60
)

type outboxDispatcherStore interface {
	ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (domain.OutboxEmail, error)
	MarkSent(ctx context.Context, id int64) error
	Release(ctx context.Context, id int64, lastError string) error
}

type Dispatcher struct {
	store  outboxDispatcherStore
	sender Sender
}

func NewDispatcher(store outboxDispatcherStore, sender Sender) *Dispatcher {
	return &Dispatcher{
		store:  store,
		sender: sender,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
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

			log.Printf("mail outbox claim failed: %v", err)
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
			TextBody: email.TextBody,
		})
		if sendErr != nil {
			if err := d.store.Release(ctx, email.ID, sendErr.Error()); err != nil {
				log.Printf("mail outbox release failed for id=%d: %v", email.ID, err)
			}
			log.Printf("mail send failed for id=%d: %v", email.ID, sendErr)
			continue
		}

		if err := d.store.MarkSent(ctx, email.ID); err != nil {
			log.Printf("mail outbox mark sent failed for id=%d: %v", email.ID, err)
		}
	}
}
