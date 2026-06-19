package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	appdb "github-release-notifier/internal/platform/db"

	"github.com/google/uuid"
)

type MessageInboxStore struct {
	db *sql.DB
}

func NewMessageInboxStore(db *sql.DB) *MessageInboxStore {
	return &MessageInboxStore{db: db}
}

func (s *MessageInboxStore) ClaimForProcessing(ctx context.Context, messageID uuid.UUID, messageType string, payloadJSON json.RawMessage, processingTimeoutSeconds int) (bool, error) {
	insertQuery := `
		insert into message_inbox (message_id, message_type, payload_json, processing_started_at)
		values ($1, $2, $3, now())
		on conflict (message_id) do nothing;
	`

	result, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, insertQuery, messageID, messageType, payloadJSON)
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected == 1 {
		return true, nil
	}

	updateQuery := `
		update message_inbox
		set processing_started_at = now()
		where message_id = $1
		  and processed_at is null
		  and (
			processing_started_at is null
			or processing_started_at < now() - make_interval(secs => $2)
		  );
	`

	result, err = appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, updateQuery, messageID, processingTimeoutSeconds)
	if err != nil {
		return false, err
	}

	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected == 1, nil
}

func (s *MessageInboxStore) MarkProcessed(ctx context.Context, messageID uuid.UUID) error {
	query := `
		update message_inbox
		set processing_started_at = null,
		    processed_at = now()
		where message_id = $1;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, messageID)
	return err
}

func (s *MessageInboxStore) ReleaseProcessing(ctx context.Context, messageID uuid.UUID) error {
	query := `
		update message_inbox
		set processing_started_at = null
		where message_id = $1
		  and processed_at is null;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, messageID)
	return err
}
