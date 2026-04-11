package storage

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

type SubscriptionStore struct {
	db *sql.DB
}

func NewSubscriptionStore(db *sql.DB) *SubscriptionStore {
	return &SubscriptionStore{db}
}

func (s *SubscriptionStore) Create(ctx context.Context, userId int64, trackedRepositoryId int64) (domain.Subscription, error) {
	query := `
		insert into subscriptions (user_id, tracked_repository_id)
		values ($1, $2)
		returning id, user_id, tracked_repository_id, confirmed, confirmation_token, cancellation_token, created_at, updated_at;
	`

	var created domain.Subscription

	err := s.db.QueryRowContext(ctx, query, userId, trackedRepositoryId).Scan(
		&created.ID,
		&created.UserID,
		&created.TrackedRepositoryID,
		&created.Confirmed,
		&created.ConfirmationToken,
		&created.CancellationToken,
		&created.CreatedAt,
		&created.UpdatedAt,
	)

	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return domain.Subscription{}, domain.ErrAlreadyExists
		}

		return domain.Subscription{}, err
	}

	return created, nil
}
