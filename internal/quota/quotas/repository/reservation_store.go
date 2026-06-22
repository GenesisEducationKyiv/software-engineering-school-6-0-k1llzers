package repository

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/quota/quotas"

	"github.com/google/uuid"
)

const rejectionReasonLimitExceeded = "subscription limit exceeded"

type ReservationStore struct {
	db *sql.DB
}

func NewReservationStore(db *sql.DB) *ReservationStore {
	return &ReservationStore{db: db}
}

func (s *ReservationStore) ReserveSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64, email string, defaultLimit int) (quotas.ReserveResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return quotas.ReserveResult{}, err
	}
	defer rollbackUnlessCommitted(tx)

	if err = s.ensureQuota(ctx, tx, email, defaultLimit); err != nil {
		return quotas.ReserveResult{}, err
	}

	if result, found, err := s.reuseExistingReservation(ctx, tx, subscriptionID); found || err != nil {
		return result, commitIfNoError(tx, err)
	}

	result, err := s.createReservationForAvailableQuota(ctx, tx, sagaID, subscriptionID, email)
	return result, commitIfNoError(tx, err)
}

func (s *ReservationStore) reuseExistingReservation(ctx context.Context, tx *sql.Tx, subscriptionID int64) (quotas.ReserveResult, bool, error) {
	existingStatus, found, err := s.findReservationStatus(ctx, tx, subscriptionID)
	if err != nil || !found {
		return quotas.ReserveResult{}, found, err
	}

	return reserveResultForExistingStatus(existingStatus), true, nil
}

func (s *ReservationStore) createReservationForAvailableQuota(ctx context.Context, tx *sql.Tx, sagaID uuid.UUID, subscriptionID int64, email string) (quotas.ReserveResult, error) {
	hasAvailableSlot, err := s.hasAvailableSlot(ctx, tx, email)
	if err != nil {
		return quotas.ReserveResult{}, err
	}

	if !hasAvailableSlot {
		return s.rejectReservation(ctx, tx, sagaID, subscriptionID, email)
	}

	return s.reserveSlot(ctx, tx, sagaID, subscriptionID, email)
}

func (s *ReservationStore) hasAvailableSlot(ctx context.Context, tx *sql.Tx, email string) (bool, error) {
	usedSlots, maxSubscriptions, err := s.lockQuota(ctx, tx, email)
	if err != nil {
		return false, err
	}

	return usedSlots < maxSubscriptions, nil
}

func (s *ReservationStore) rejectReservation(ctx context.Context, tx *sql.Tx, sagaID uuid.UUID, subscriptionID int64, email string) (quotas.ReserveResult, error) {
	if err := s.createRejectedReservation(ctx, tx, sagaID, subscriptionID, email, rejectionReasonLimitExceeded); err != nil {
		return quotas.ReserveResult{}, err
	}

	return quotas.ReserveResult{Reserved: false, RejectionReason: rejectionReasonLimitExceeded}, nil
}

func (s *ReservationStore) reserveSlot(ctx context.Context, tx *sql.Tx, sagaID uuid.UUID, subscriptionID int64, email string) (quotas.ReserveResult, error) {
	if err := s.createReservedReservation(ctx, tx, sagaID, subscriptionID, email); err != nil {
		return quotas.ReserveResult{}, err
	}

	if err := s.incrementUsedSlots(ctx, tx, email); err != nil {
		return quotas.ReserveResult{}, err
	}

	return quotas.ReserveResult{Reserved: true}, nil
}

func (s *ReservationStore) CommitSlot(ctx context.Context, subscriptionID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var status quotas.ReservationStatus
	err = tx.QueryRowContext(
		ctx,
		`select status from quota_reservations where subscription_id = $1 for update`,
		subscriptionID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return quotas.ErrReservationNotFound
		}

		return err
	}

	switch status {
	case quotas.ReservationStatusCommitted:
		return tx.Commit()
	case quotas.ReservationStatusReserved:
		_, err = tx.ExecContext(
			ctx,
			`
				update quota_reservations
				set status = 'committed',
					updated_at = now()
				where subscription_id = $1;
			`,
			subscriptionID,
		)
		if err != nil {
			return err
		}

		return tx.Commit()
	default:
		return quotas.ErrReservationCannotBeCommitted
	}
}

func (s *ReservationStore) ReleaseSlot(ctx context.Context, subscriptionID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var email string
	var status quotas.ReservationStatus
	err = tx.QueryRowContext(
		ctx,
		`select email, status from quota_reservations where subscription_id = $1 for update`,
		subscriptionID,
	).Scan(&email, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.Commit()
			return err
		}

		return err
	}

	if status == quotas.ReservationStatusReleased || status == quotas.ReservationStatusRejected {
		err = tx.Commit()
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`
			update quota_reservations
			set status = 'released',
				updated_at = now()
			where subscription_id = $1;
		`,
		subscriptionID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`
			update subscription_quotas
			set used_slots = greatest(used_slots - 1, 0),
				updated_at = now()
			where email = $1;
		`,
		email,
	)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

func (s *ReservationStore) ensureQuota(ctx context.Context, tx *sql.Tx, email string, defaultLimit int) error {
	_, err := tx.ExecContext(
		ctx,
		`
			insert into subscription_quotas (email, max_subscriptions)
			values ($1, $2)
			on conflict (email) do nothing;
		`,
		email,
		defaultLimit,
	)
	return err
}

func (s *ReservationStore) findReservationStatus(ctx context.Context, tx *sql.Tx, subscriptionID int64) (quotas.ReservationStatus, bool, error) {
	var status quotas.ReservationStatus
	err := tx.QueryRowContext(
		ctx,
		`select status from quota_reservations where subscription_id = $1 for update`,
		subscriptionID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}

		return "", false, err
	}

	return status, true, nil
}

func (s *ReservationStore) lockQuota(ctx context.Context, tx *sql.Tx, email string) (int, int, error) {
	var usedSlots int
	var maxSubscriptions int
	err := tx.QueryRowContext(
		ctx,
		`select used_slots, max_subscriptions from subscription_quotas where email = $1 for update`,
		email,
	).Scan(&usedSlots, &maxSubscriptions)
	return usedSlots, maxSubscriptions, err
}

func (s *ReservationStore) createRejectedReservation(ctx context.Context, tx *sql.Tx, sagaID uuid.UUID, subscriptionID int64, email string, reason string) error {
	_, err := tx.ExecContext(
		ctx,
		`
			insert into quota_reservations (saga_id, subscription_id, email, status, rejection_reason)
			values ($1, $2, $3, 'rejected', $4);
		`,
		sagaID,
		subscriptionID,
		email,
		reason,
	)
	return err
}

func (s *ReservationStore) createReservedReservation(ctx context.Context, tx *sql.Tx, sagaID uuid.UUID, subscriptionID int64, email string) error {
	_, err := tx.ExecContext(
		ctx,
		`
			insert into quota_reservations (saga_id, subscription_id, email, status)
			values ($1, $2, $3, 'reserved');
		`,
		sagaID,
		subscriptionID,
		email,
	)
	return err
}

func (s *ReservationStore) incrementUsedSlots(ctx context.Context, tx *sql.Tx, email string) error {
	_, err := tx.ExecContext(
		ctx,
		`
			update subscription_quotas
			set used_slots = used_slots + 1,
				updated_at = now()
			where email = $1;
		`,
		email,
	)
	return err
}

func reserveResultForExistingStatus(status quotas.ReservationStatus) quotas.ReserveResult {
	switch status {
	case quotas.ReservationStatusReserved, quotas.ReservationStatusCommitted:
		return quotas.ReserveResult{Reserved: true}
	case quotas.ReservationStatusRejected:
		return quotas.ReserveResult{Reserved: false, RejectionReason: rejectionReasonLimitExceeded}
	default:
		return quotas.ReserveResult{Reserved: false, RejectionReason: "reservation already released"}
	}
}

func commitIfNoError(tx *sql.Tx, err error) error {
	if err != nil {
		return err
	}

	return tx.Commit()
}

func rollbackUnlessCommitted(tx *sql.Tx) {
	_ = tx.Rollback()
}
