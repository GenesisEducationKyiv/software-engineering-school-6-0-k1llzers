package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github-release-notifier/internal/app/subscriptions"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/shared"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type SagaStore struct {
	db *sql.DB
}

func NewSagaStore(db *sql.DB) *SagaStore {
	return &SagaStore{db: db}
}

func (s *SagaStore) Get(ctx context.Context, sagaID uuid.UUID) (subscriptions.Saga, error) {
	query := `
		select id, subscription_id, operation, status, attempts, last_error, processing_started_at, next_retry_at, created_at, updated_at
		from subscription_sagas
		where id = $1;
	`

	return s.scanSagaRow(appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, sagaID))
}

func (s *SagaStore) Create(ctx context.Context, saga subscriptions.Saga) error {
	query := `
		insert into subscription_sagas (id, subscription_id, operation, status)
		values ($1, $2, $3, $4);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		saga.ID,
		saga.SubscriptionID,
		saga.Operation,
		subscriptions.SagaStatusPending,
	)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return subscriptions.ErrAlreadyExists
		}

		return err
	}

	return nil
}

func (s *SagaStore) ClaimByID(ctx context.Context, sagaID uuid.UUID, processingTimeoutSeconds int) (subscriptions.Saga, error) {
	query := `
		update subscription_sagas
		set status = case
				when status in ($2, $3, $4) then $4
				else status
			end,
			processing_started_at = now(),
			updated_at = now()
		where id = $1
		  and (
			status = $2
			or (status = $3 and (next_retry_at is null or next_retry_at <= now()))
			or (status = $4 and processing_started_at < now() - make_interval(secs => $5))
			or (status = $6 and (next_retry_at is null or next_retry_at <= now()))
			or (status = $7 and (next_retry_at is null or next_retry_at <= now()))
		  )
		  and (
			processing_started_at is null
			or processing_started_at < now() - make_interval(secs => $5)
		  )
		returning id, subscription_id, operation, status, attempts, last_error, processing_started_at, next_retry_at, created_at, updated_at;
	`

	return s.scanSagaRow(s.db.QueryRowContext(
		ctx,
		query,
		sagaID,
		subscriptions.SagaStatusPending,
		subscriptions.SagaStatusFailed,
		subscriptions.SagaStatusRunning,
		processingTimeoutSeconds,
		subscriptions.SagaStatusQuotaCommitted,
		subscriptions.SagaStatusCompensating,
	))
}

func (s *SagaStore) ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (subscriptions.Saga, error) {
	query := `
		with candidate as (
			select id
			from subscription_sagas
			where (
				status = $1
				or (status = $2 and (next_retry_at is null or next_retry_at <= now()))
				or (status = $3 and processing_started_at < now() - make_interval(secs => $4))
				or (status = $5 and (next_retry_at is null or next_retry_at <= now()))
				or (status = $6 and (next_retry_at is null or next_retry_at <= now()))
			)
			  and (
				processing_started_at is null
				or processing_started_at < now() - make_interval(secs => $4)
			  )
			order by updated_at, created_at
			for update skip locked
			limit 1
		)
		update subscription_sagas s
		set status = case
				when s.status in ($1, $2, $3) then $3
				else s.status
			end,
			processing_started_at = now(),
			updated_at = now()
		from candidate
		where s.id = candidate.id
		returning s.id, s.subscription_id, s.operation, s.status, s.attempts, s.last_error, s.processing_started_at, s.next_retry_at, s.created_at, s.updated_at;
	`

	return s.scanSagaRow(s.db.QueryRowContext(
		ctx,
		query,
		subscriptions.SagaStatusPending,
		subscriptions.SagaStatusFailed,
		subscriptions.SagaStatusRunning,
		processingTimeoutSeconds,
		subscriptions.SagaStatusQuotaCommitted,
		subscriptions.SagaStatusCompensating,
	))
}

func (s *SagaStore) MarkQuotaCommitted(ctx context.Context, sagaID uuid.UUID) error {
	query := `
		update subscription_sagas
		set status = $2,
			last_error = null,
			next_retry_at = null,
			updated_at = now()
		where id = $1
		  and status in ($3, $2);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		sagaID,
		subscriptions.SagaStatusQuotaCommitted,
		subscriptions.SagaStatusRunning,
	)
	return err
}

func (s *SagaStore) MarkCompleted(ctx context.Context, sagaID uuid.UUID) error {
	query := `
		update subscription_sagas
		set status = $2,
			last_error = null,
			processing_started_at = null,
			next_retry_at = null,
			updated_at = now()
		where id = $1
		  and status not in ($3, $4, $5, $6);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		sagaID,
		subscriptions.SagaStatusCompleted,
		subscriptions.SagaStatusCompleted,
		subscriptions.SagaStatusCompensated,
		subscriptions.SagaStatusDead,
		subscriptions.SagaStatusRejected,
	)
	return err
}

func (s *SagaStore) MarkRejected(ctx context.Context, saga subscriptions.Saga, reason string) error {
	query := `
		update subscription_sagas
		set status = $2,
			attempts = attempts + 1,
			last_error = $3,
			processing_started_at = null,
			next_retry_at = null,
			updated_at = now()
		where id = $1
		  and status not in ($4, $5, $6, $7);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		saga.ID,
		subscriptions.SagaStatusRejected,
		reason,
		subscriptions.SagaStatusCompleted,
		subscriptions.SagaStatusCompensated,
		subscriptions.SagaStatusDead,
		subscriptions.SagaStatusRejected,
	)
	return err
}

func (s *SagaStore) MarkDead(ctx context.Context, saga subscriptions.Saga, cause error) error {
	query := `
		update subscription_sagas
		set status = $2,
			attempts = attempts + 1,
			last_error = $3,
			processing_started_at = null,
			next_retry_at = null,
			updated_at = now()
		where id = $1
		  and status not in ($4, $5, $6, $7);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		saga.ID,
		subscriptions.SagaStatusDead,
		cause.Error(),
		subscriptions.SagaStatusCompleted,
		subscriptions.SagaStatusCompensated,
		subscriptions.SagaStatusDead,
		subscriptions.SagaStatusRejected,
	)
	return err
}

func (s *SagaStore) MarkFailed(ctx context.Context, saga subscriptions.Saga, cause error, nextRetryAt time.Time, maxAttempts int) (subscriptions.SagaStatus, error) {
	query := `
		update subscription_sagas
		set status = case
				when status = $10 then $10
				when attempts + 1 >= $4 then $5
				else $2
			end,
			attempts = attempts + 1,
			last_error = $3,
			processing_started_at = null,
			next_retry_at = case
				when status = $10 then $6
				when attempts + 1 >= $4 then null
				else $6
			end,
			updated_at = now()
		where id = $1
		  and status not in ($7, $8, $9, $11)
		returning status;
	`

	var status subscriptions.SagaStatus
	err := appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(
		ctx,
		query,
		saga.ID,
		subscriptions.SagaStatusFailed,
		cause.Error(),
		maxAttempts,
		subscriptions.SagaStatusDead,
		nextRetryAt,
		subscriptions.SagaStatusCompleted,
		subscriptions.SagaStatusCompensated,
		subscriptions.SagaStatusDead,
		subscriptions.SagaStatusQuotaCommitted,
		subscriptions.SagaStatusRejected,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.currentStatus(ctx, saga.ID)
		}

		return "", err
	}

	return status, nil
}

func (s *SagaStore) MarkCompensating(ctx context.Context, saga subscriptions.Saga, cause error) error {
	query := `
		update subscription_sagas
		set status = $2,
			attempts = attempts + 1,
			last_error = $3,
			next_retry_at = null,
			updated_at = now()
		where id = $1
		  and status not in ($4, $5, $6, $7);
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		saga.ID,
		subscriptions.SagaStatusCompensating,
		cause.Error(),
		subscriptions.SagaStatusCompleted,
		subscriptions.SagaStatusCompensated,
		subscriptions.SagaStatusDead,
		subscriptions.SagaStatusRejected,
	)
	return err
}

func (s *SagaStore) MarkCompensationFailed(ctx context.Context, saga subscriptions.Saga, cause error, nextRetryAt time.Time, maxAttempts int) error {
	query := `
		update subscription_sagas
		set status = case
				when attempts + 1 >= $5 then $6
				else status
			end,
			attempts = attempts + 1,
			last_error = $2,
			processing_started_at = null,
			next_retry_at = case
				when attempts + 1 >= $5 then null
				else $3
			end,
			updated_at = now()
		where id = $1
		  and status = $4;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		saga.ID,
		cause.Error(),
		nextRetryAt,
		subscriptions.SagaStatusCompensating,
		maxAttempts,
		subscriptions.SagaStatusDead,
	)
	return err
}

func (s *SagaStore) MarkCompensated(ctx context.Context, sagaID uuid.UUID) error {
	query := `
		update subscription_sagas
		set status = $2,
			last_error = null,
			processing_started_at = null,
			next_retry_at = null,
			updated_at = now()
		where id = $1
		  and status = $3;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(
		ctx,
		query,
		sagaID,
		subscriptions.SagaStatusCompensated,
		subscriptions.SagaStatusCompensating,
	)
	return err
}

func (s *SagaStore) scanSagaRow(row *sql.Row) (subscriptions.Saga, error) {
	var saga subscriptions.Saga
	var lastError sql.NullString
	var processingStartedAt sql.NullTime
	var nextRetryAt sql.NullTime

	err := row.Scan(
		&saga.ID,
		&saga.SubscriptionID,
		&saga.Operation,
		&saga.Status,
		&saga.Attempts,
		&lastError,
		&processingStartedAt,
		&nextRetryAt,
		&saga.CreatedAt,
		&saga.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return subscriptions.Saga{}, shared.ErrNotFound
		}

		return subscriptions.Saga{}, err
	}

	if lastError.Valid {
		value := lastError.String
		saga.LastError = &value
	}

	if processingStartedAt.Valid {
		value := processingStartedAt.Time
		saga.ProcessingStartedAt = &value
	}

	if nextRetryAt.Valid {
		value := nextRetryAt.Time
		saga.NextRetryAt = &value
	}

	return saga, nil
}

func (s *SagaStore) currentStatus(ctx context.Context, sagaID uuid.UUID) (subscriptions.SagaStatus, error) {
	var status subscriptions.SagaStatus
	err := appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(
		ctx,
		`select status from subscription_sagas where id = $1`,
		sagaID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", shared.ErrNotFound
		}

		return "", err
	}

	return status, nil
}
