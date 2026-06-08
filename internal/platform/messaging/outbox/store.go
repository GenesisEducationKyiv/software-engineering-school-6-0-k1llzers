package outbox

import (
	"context"
	"database/sql"
	"errors"

	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/shared"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, message Message) error {
	query := `
		insert into integration_outbox (message_id, message_type, payload_json)
		values ($1, $2, $3);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		message.MessageID,
		message.MessageType,
		message.PayloadJSON,
	)
	return err
}

func (s *Store) ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (Message, error) {
	query := `
		with candidate as (
			select id
			from integration_outbox
			where published_at is null
			  and (
				processing_started_at is null
				or processing_started_at < now() - make_interval(secs => $1)
			  )
			order by id
			for update skip locked
			limit 1
		)
		update integration_outbox o
		set processing_started_at = now()
		from candidate
		where o.id = candidate.id
		returning o.id, o.message_id, o.message_type, o.payload_json, o.processing_started_at, o.published_at, o.created_at;
	`

	var result Message
	err := s.db.QueryRowContext(ctx, query, processingTimeoutSeconds).Scan(
		&result.ID,
		&result.MessageID,
		&result.MessageType,
		&result.PayloadJSON,
		&result.ProcessingStartedAt,
		&result.PublishedAt,
		&result.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, shared.ErrNotFound
		}

		return Message{}, err
	}

	return result, nil
}

func (s *Store) MarkPublished(ctx context.Context, id int64) error {
	query := `
		update integration_outbox
		set published_at = now(),
			processing_started_at = null
		where id = $1;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, id)
	return err
}

func (s *Store) Release(ctx context.Context, id int64) error {
	query := `
		update integration_outbox
		set processing_started_at = null
		where id = $1;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, id)
	return err
}
