package storage

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/domain"
)

type OutboxStore struct {
	executor executor
}

func NewOutboxStore(db *sql.DB) *OutboxStore {
	return &OutboxStore{executor: db}
}

func (s *OutboxStore) WithTx(tx *sql.Tx) *OutboxStore {
	return &OutboxStore{executor: tx}
}

func (s *OutboxStore) Create(ctx context.Context, recipientEmail string, email domain.OutboxEmail) error {
	query := `
		insert into mail_outbox (recipient_email, subject, html_body, text_body)
		values ($1, $2, $3, $4);
	`

	_, err := s.executor.ExecContext(ctx, query, recipientEmail, email.Subject, email.HTMLBody, email.TextBody)
	return err
}

func (s *OutboxStore) ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (domain.OutboxEmail, error) {
	query := `
		with candidate as (
			select id
			from mail_outbox
			where sent_at is null
			  and (
				processing_started_at is null
				or processing_started_at < now() - make_interval(secs => $1)
			  )
			order by id
			for update skip locked
			limit 1
		)
		update mail_outbox o
		set processing_started_at = now(),
			attempts = attempts + 1,
			updated_at = now()
		from candidate
		where o.id = candidate.id
		returning o.id, o.recipient_email, o.subject, o.html_body, o.text_body, o.attempts, o.processing_started_at, o.sent_at, o.last_error, o.created_at, o.updated_at;
	`

	var result domain.OutboxEmail
	err := s.executor.QueryRowContext(ctx, query, processingTimeoutSeconds).Scan(
		&result.ID,
		&result.RecipientEmail,
		&result.Subject,
		&result.HTMLBody,
		&result.TextBody,
		&result.Attempts,
		&result.ProcessingStartedAt,
		&result.SentAt,
		&result.LastError,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.OutboxEmail{}, domain.ErrNotFound
		}

		return domain.OutboxEmail{}, err
	}

	return result, nil
}

func (s *OutboxStore) MarkSent(ctx context.Context, id int64) error {
	query := `
		update mail_outbox
		set sent_at = now(),
			processing_started_at = null,
			last_error = null,
			updated_at = now()
		where id = $1;
	`

	_, err := s.executor.ExecContext(ctx, query, id)
	return err
}

func (s *OutboxStore) Release(ctx context.Context, id int64, lastError string) error {
	query := `
		update mail_outbox
		set processing_started_at = null,
			last_error = $2,
			updated_at = now()
		where id = $1;
	`

	_, err := s.executor.ExecContext(ctx, query, id, lastError)
	return err
}
