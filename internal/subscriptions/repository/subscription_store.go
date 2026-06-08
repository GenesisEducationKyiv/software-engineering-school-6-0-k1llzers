package repository

import (
	"context"
	"database/sql"
	"errors"

	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/shared"
	"github-release-notifier/internal/subscriptions"

	"github.com/jackc/pgx/v5/pgconn"
)

type SubscriptionStore struct {
	db *sql.DB
}

func NewSubscriptionStore(db *sql.DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

func (s *SubscriptionStore) Create(ctx context.Context, userID int64, trackedRepositoryID int64) (subscriptions.Subscription, error) {
	query := `
		insert into subscriptions (user_id, tracked_repository_id)
		values ($1, $2)
		returning id, user_id, tracked_repository_id, confirmed, confirmation_token, cancellation_token, created_at, updated_at;
	`

	var created subscriptions.Subscription

	err := appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, userID, trackedRepositoryID).Scan(
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
			return subscriptions.Subscription{}, subscriptions.ErrAlreadyExists
		}

		return subscriptions.Subscription{}, err
	}

	return created, nil
}

func (s *SubscriptionStore) SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error {
	query := `
		update subscriptions
		set confirmed = true,
			updated_at = now()
		where confirmation_token = $1 and confirmed = false;
	`

	result, err := s.db.ExecContext(ctx, query, confirmationToken)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `select exists(select 1 from subscriptions where confirmation_token = $1)`, confirmationToken).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return subscriptions.ErrInvalidToken
		}

		return shared.ErrNotFound
	}

	return nil
}

func (s *SubscriptionStore) DeleteByCancellationToken(ctx context.Context, cancellationToken string) error {
	query := `
		delete from subscriptions 
		where cancellation_token=$1;
	`

	result, err := s.db.ExecContext(ctx, query, cancellationToken)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return shared.ErrNotFound
	}

	return nil
}

func (s *SubscriptionStore) ListByEmail(ctx context.Context, email string) ([]subscriptions.SubscriptionView, error) {
	query := `
		select
			u.email,
			tr.owner || '/' || tr.name as repo,
			s.confirmed,
			coalesce(tr.last_seen_tag, '')
		from subscriptions s
		join users u on u.id = s.user_id
		join tracked_repositories tr on tr.id = s.tracked_repository_id
		where u.email = $1
		order by tr.owner, tr.name;
	`

	rows, err := s.db.QueryContext(ctx, query, email)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var subscriptionsView []subscriptions.SubscriptionView
	for rows.Next() {
		var item subscriptions.SubscriptionView
		if err := rows.Scan(
			&item.Email,
			&item.Repo,
			&item.Confirmed,
			&item.LastSeenTag,
		); err != nil {
			return nil, err
		}

		subscriptionsView = append(subscriptionsView, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return subscriptionsView, nil
}
