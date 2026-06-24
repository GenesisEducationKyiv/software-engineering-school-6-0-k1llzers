package repository

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/app/subscriptions"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/shared"

	"github.com/jackc/pgx/v5/pgconn"
)

type SubscriptionStore struct {
	db *sql.DB
}

func NewSubscriptionStore(db *sql.DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

func (s *SubscriptionStore) CreatePending(ctx context.Context, userID int64, trackedRepositoryID int64) (subscriptions.Subscription, error) {
	query := `
		insert into subscriptions (user_id, tracked_repository_id)
		values ($1, $2)
		returning
			id,
			user_id,
			tracked_repository_id,
			confirmed,
			confirmation_token,
			cancellation_token,
			created_at,
			updated_at;
	`

	var created subscriptions.Subscription

	err := scanSubscription(
		appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, userID, trackedRepositoryID),
		&created,
	)

	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return subscriptions.Subscription{}, subscriptions.ErrAlreadyExists
		}

		return subscriptions.Subscription{}, err
	}

	return created, nil
}

func (s *SubscriptionStore) ConfirmByToken(ctx context.Context, confirmationToken string) error {
	query := `
		update subscriptions
		set confirmed = true,
			updated_at = now()
		where confirmation_token = $1
		  and confirmed = false;
	`

	executor := appdb.NewQueryExecutor(ctx, s.db)
	result, err := executor.ExecContext(
		ctx,
		query,
		confirmationToken,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		var exists bool
		if err := executor.QueryRowContext(ctx, `select exists(select 1 from subscriptions where confirmation_token = $1)`, confirmationToken).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return subscriptions.ErrInvalidToken
		}

		return shared.ErrNotFound
	}

	return nil
}

func (s *SubscriptionStore) SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error {
	return s.ConfirmByToken(ctx, confirmationToken)
}

func (s *SubscriptionStore) FindByCancellationToken(ctx context.Context, cancellationToken string) (subscriptions.Subscription, error) {
	query := `
		select
			id,
			user_id,
			tracked_repository_id,
			confirmed,
			confirmation_token,
			cancellation_token,
			created_at,
			updated_at
		from subscriptions
		where cancellation_token = $1;
	`

	var found subscriptions.Subscription
	if err := scanSubscription(appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, cancellationToken), &found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return subscriptions.Subscription{}, shared.ErrNotFound
		}

		return subscriptions.Subscription{}, err
	}

	return found, nil
}

func (s *SubscriptionStore) CancelByCancellationToken(ctx context.Context, cancellationToken string) error {
	query := `
		delete from subscriptions
		where cancellation_token = $1;
	`

	result, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, cancellationToken)
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

func (s *SubscriptionStore) DeleteByCancellationToken(ctx context.Context, cancellationToken string) error {
	return s.CancelByCancellationToken(ctx, cancellationToken)
}

func (s *SubscriptionStore) DeleteByID(ctx context.Context, subscriptionID int64) error {
	query := `
		delete from subscriptions
		where id = $1;
	`

	result, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, subscriptionID)
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

func (s *SubscriptionStore) GetSubscriptionDetailsForSaga(ctx context.Context, subscriptionID int64) (subscriptions.SagaSubscriptionDetails, error) {
	query := `
		select
			s.id,
			s.user_id,
			s.tracked_repository_id,
			s.confirmed,
			s.confirmation_token,
			s.cancellation_token,
			s.created_at,
			s.updated_at,
			u.email,
			tr.owner || '/' || tr.name as repo
		from subscriptions s
		join users u on u.id = s.user_id
		join tracked_repositories tr on tr.id = s.tracked_repository_id
		where s.id = $1;
	`

	var details subscriptions.SagaSubscriptionDetails
	if err := scanSubscriptionDetails(appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, subscriptionID), &details); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return subscriptions.SagaSubscriptionDetails{}, shared.ErrNotFound
		}

		return subscriptions.SagaSubscriptionDetails{}, err
	}

	return details, nil
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
		  and s.confirmed = true
		order by tr.owner, tr.name;
	`

	rows, err := appdb.NewQueryExecutor(ctx, s.db).QueryContext(ctx, query, email)
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

func scanSubscriptionDetails(scanner subscriptionScanner, destination *subscriptions.SagaSubscriptionDetails) error {
	if err := scanner.Scan(
		&destination.ID,
		&destination.UserID,
		&destination.TrackedRepositoryID,
		&destination.Confirmed,
		&destination.ConfirmationToken,
		&destination.CancellationToken,
		&destination.CreatedAt,
		&destination.UpdatedAt,
		&destination.Email,
		&destination.RepositoryFullName,
	); err != nil {
		return err
	}

	return nil
}

type subscriptionScanner interface {
	Scan(dest ...any) error
}

func scanSubscription(scanner subscriptionScanner, destination *subscriptions.Subscription) error {
	if err := scanner.Scan(
		&destination.ID,
		&destination.UserID,
		&destination.TrackedRepositoryID,
		&destination.Confirmed,
		&destination.ConfirmationToken,
		&destination.CancellationToken,
		&destination.CreatedAt,
		&destination.UpdatedAt,
	); err != nil {
		return err
	}

	return nil
}
