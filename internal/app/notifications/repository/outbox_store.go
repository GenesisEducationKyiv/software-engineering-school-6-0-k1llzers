package repository

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/app/notifications"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/shared"
)

type OutboxStore struct {
	db *sql.DB
}

func NewOutboxStore(db *sql.DB) *OutboxStore {
	return &OutboxStore{db: db}
}

func (s *OutboxStore) Create(ctx context.Context, recipientEmail string, email notifications.Email) error {
	query := `
		insert into mail_outbox (recipient_email, subject, html_body)
		values ($1, $2, $3);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, recipientEmail, email.Subject, email.HTMLBody)
	return err
}

func (s *OutboxStore) ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (notifications.Email, error) {
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
		returning o.id, o.recipient_email, o.subject, o.html_body, o.attempts, o.processing_started_at, o.sent_at, o.last_error, o.created_at, o.updated_at;
	`

	var result notifications.Email
	err := s.db.QueryRowContext(ctx, query, processingTimeoutSeconds).Scan(
		&result.ID,
		&result.RecipientEmail,
		&result.Subject,
		&result.HTMLBody,
		&result.Attempts,
		&result.ProcessingStartedAt,
		&result.SentAt,
		&result.LastError,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notifications.Email{}, shared.ErrNotFound
		}

		return notifications.Email{}, err
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

	_, err := s.db.ExecContext(ctx, query, id)
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

	_, err := s.db.ExecContext(ctx, query, id, lastError)
	return err
}
