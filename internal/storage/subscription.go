package storage

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

type SubscriptionStore struct {
	executor executor
}

func NewSubscriptionStore(db *sql.DB) *SubscriptionStore {
	return &SubscriptionStore{executor: db}
}

func (s *SubscriptionStore) WithTx(tx *sql.Tx) *SubscriptionStore {
	return &SubscriptionStore{executor: tx}
}

func (s *SubscriptionStore) Create(ctx context.Context, userId int64, trackedRepositoryId int64) (domain.Subscription, error) {
	query := `
		insert into subscriptions (user_id, tracked_repository_id)
		values ($1, $2)
		returning id, user_id, tracked_repository_id, confirmed, confirmation_token, cancellation_token, created_at, updated_at;
	`

	var created domain.Subscription

	err := s.executor.QueryRowContext(ctx, query, userId, trackedRepositoryId).Scan(
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

func (s *SubscriptionStore) SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error {
	query := `
		update subscriptions 
			set confirmed=true 
		where confirmation_token=$1 and confirmed=false;
	`

	result, err := s.executor.ExecContext(ctx, query, confirmationToken)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (s *SubscriptionStore) DeleteByCancellationToken(ctx context.Context, cancellationToken string) error {
	query := `
		delete from subscriptions 
		where cancellation_token=$1;
	`

	result, err := s.executor.ExecContext(ctx, query, cancellationToken)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}
