//go:build integration

package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/db/test"
	"github-release-notifier/internal/quota/quotas"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReservationStore_ReserveSlotCreatesReservation(t *testing.T) {
	db := setupQuotaTestDB(t)
	store := NewReservationStore(db)
	ctx := context.Background()
	sagaID := uuid.New()

	result, err := store.ReserveSlot(ctx, sagaID, 10, test.NewTestEmail(), 5)

	require.NoError(t, err)
	require.True(t, result.Reserved)
	requireQuotaUsage(t, db, 10, sagaID, quotas.ReservationStatusReserved, 1)
}

func TestReservationStore_ReserveSlotIsIdempotentForSameSaga(t *testing.T) {
	db := setupQuotaTestDB(t)
	store := NewReservationStore(db)
	ctx := context.Background()
	email := test.NewTestEmail()
	sagaID := uuid.New()

	firstResult, err := store.ReserveSlot(ctx, sagaID, 10, email, 5)
	require.NoError(t, err)
	require.True(t, firstResult.Reserved)

	secondResult, err := store.ReserveSlot(ctx, sagaID, 10, email, 5)

	require.NoError(t, err)
	require.True(t, secondResult.Reserved)
	requireQuotaUsage(t, db, 10, sagaID, quotas.ReservationStatusReserved, 1)
}

func TestReservationStore_ReserveSlotRejectsDifferentSagaForExistingSubscription(t *testing.T) {
	db := setupQuotaTestDB(t)
	store := NewReservationStore(db)
	ctx := context.Background()
	email := test.NewTestEmail()
	firstSagaID := uuid.New()

	_, err := store.ReserveSlot(ctx, firstSagaID, 10, email, 5)
	require.NoError(t, err)

	_, err = store.ReserveSlot(ctx, uuid.New(), 10, email, 5)

	require.ErrorIs(t, err, quotas.ErrReservationSagaMismatch)
	requireQuotaUsage(t, db, 10, firstSagaID, quotas.ReservationStatusReserved, 1)
}

func TestReservationStore_ReleaseSlotIsIdempotentForSameSaga(t *testing.T) {
	db := setupQuotaTestDB(t)
	store := NewReservationStore(db)
	ctx := context.Background()
	email := test.NewTestEmail()
	reserveSagaID := uuid.New()
	releaseSagaID := uuid.New()

	_, err := store.ReserveSlot(ctx, reserveSagaID, 10, email, 5)
	require.NoError(t, err)

	err = store.ReleaseSlot(ctx, releaseSagaID, 10)
	require.NoError(t, err)
	err = store.ReleaseSlot(ctx, releaseSagaID, 10)
	require.NoError(t, err)

	requireQuotaUsage(t, db, 10, reserveSagaID, quotas.ReservationStatusReleased, 0)
}

func TestReservationStore_ReleaseSlotRejectsDifferentSagaForReleasedReservation(t *testing.T) {
	db := setupQuotaTestDB(t)
	store := NewReservationStore(db)
	ctx := context.Background()
	email := test.NewTestEmail()
	reserveSagaID := uuid.New()

	_, err := store.ReserveSlot(ctx, reserveSagaID, 10, email, 5)
	require.NoError(t, err)
	err = store.ReleaseSlot(ctx, uuid.New(), 10)
	require.NoError(t, err)

	err = store.ReleaseSlot(ctx, uuid.New(), 10)

	require.ErrorIs(t, err, quotas.ErrReservationSagaMismatch)
	requireQuotaUsage(t, db, 10, reserveSagaID, quotas.ReservationStatusReleased, 0)
}

func setupQuotaTestDB(t *testing.T) *sql.DB {
	t.Helper()

	_, db := test.SetupTestPostgres(t)
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "migrations", "quota"))
	require.NoError(t, appdb.RunMigrations(context.Background(), db, migrationsDir))
	return db
}

func requireQuotaUsage(t *testing.T, db *sql.DB, subscriptionID int64, sagaID uuid.UUID, expectedStatus quotas.ReservationStatus, expectedUsedSlots int) {
	t.Helper()

	var actualSagaID uuid.UUID
	var actualStatus quotas.ReservationStatus
	var actualUsedSlots int
	err := db.QueryRowContext(
		context.Background(),
		`
			select r.reservation_saga_id, r.status, q.used_slots
			from quota_reservations r
			join subscription_quotas q on q.email = r.email
			where r.subscription_id = $1
		`,
		subscriptionID,
	).Scan(&actualSagaID, &actualStatus, &actualUsedSlots)
	require.NoError(t, err)
	require.Equal(t, sagaID, actualSagaID)
	require.Equal(t, expectedStatus, actualStatus)
	require.Equal(t, expectedUsedSlots, actualUsedSlots)
}
