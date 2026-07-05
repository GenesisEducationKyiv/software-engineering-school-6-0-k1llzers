//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	releasetrackingrepo "github-release-notifier/internal/app/release_tracking/repository"
	"github-release-notifier/internal/app/subscriptions"
	dbtx "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/db/test"
	"github-release-notifier/internal/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionStore_CreatePending(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	created, err := subscriptionStore.CreatePending(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, user.ID, created.UserID)
	require.Equal(t, trackedRepository.ID, created.TrackedRepositoryID)
	require.False(t, created.Confirmed)
	require.NotEqual(t, created.ConfirmationToken.String(), created.CancellationToken.String())
	require.False(t, created.CreatedAt.IsZero())
	require.False(t, created.UpdatedAt.IsZero())
}

func TestSubscriptionStore_Create_UsesTransaction(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()
	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	repository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	tx := test.BeginTestTx(t, db)
	created, err := subscriptionStore.CreatePending(dbtx.WithTransactionContext(ctx, tx), user.ID, repository.ID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var count int
	err = db.QueryRowContext(ctx, `select count(*) from subscriptions where id = $1`, created.ID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSubscriptionStore_Create_DuplicateSubscription(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	_, err = subscriptionStore.CreatePending(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	_, err = subscriptionStore.CreatePending(ctx, user.ID, trackedRepository.ID)
	require.ErrorIs(t, err, subscriptions.ErrAlreadyExists)
}

func TestSubscriptionStore_DeleteByID(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	created, err := subscriptionStore.CreatePending(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	err = subscriptionStore.DeleteByID(ctx, created.ID)
	require.NoError(t, err)
	requireSubscriptionMissing(t, db, created.ID)
}

func TestSubscriptionStore_ConfirmByToken(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	created := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, trackedRepository.ID)

	err = subscriptionStore.ConfirmByToken(ctx, created.ConfirmationToken.String())
	require.NoError(t, err)

	var confirmed bool
	err = db.QueryRowContext(ctx, `select confirmed from subscriptions where id = $1`, created.ID).Scan(&confirmed)
	require.NoError(t, err)
	require.True(t, confirmed)
	requireSubscribeSagaStatus(t, db, created.ID, subscriptions.SagaStatusCompleted)
}

func TestSubscriptionStore_ConfirmByToken_ConfirmsPendingSubscription(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	created, err := subscriptionStore.CreatePending(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	err = subscriptionStore.ConfirmByToken(ctx, created.ConfirmationToken.String())
	require.NoError(t, err)
	requireNoSubscribeSaga(t, db, created.ID)

	var confirmed bool
	err = db.QueryRowContext(ctx, `select confirmed from subscriptions where id = $1`, created.ID).Scan(&confirmed)
	require.NoError(t, err)
	require.True(t, confirmed)
}

func TestSubscriptionStore_ConfirmByToken_ReturnsInvalidTokenWhenAlreadyConfirmed(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	created := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, trackedRepository.ID)

	err = subscriptionStore.ConfirmByToken(ctx, created.ConfirmationToken.String())
	require.NoError(t, err)

	err = subscriptionStore.ConfirmByToken(ctx, created.ConfirmationToken.String())
	require.ErrorIs(t, err, subscriptions.ErrInvalidToken)
}

func TestSubscriptionStore_ConfirmByToken_NotFound(t *testing.T) {
	db := test.SetupTestDB(t)
	subscriptionStore := NewSubscriptionStore(db)

	err := subscriptionStore.ConfirmByToken(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestSubscriptionStore_CancelByCancellationToken(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	created := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, trackedRepository.ID)

	err = subscriptionStore.CancelByCancellationToken(ctx, created.CancellationToken.String())
	require.NoError(t, err)
	requireSubscriptionMissing(t, db, created.ID)
}

func TestSubscriptionStore_CancelByCancellationToken_NotFound(t *testing.T) {
	db := test.SetupTestDB(t)
	subscriptionStore := NewSubscriptionStore(db)

	err := subscriptionStore.CancelByCancellationToken(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestSubscriptionStore_DeletedSubscriptionDoesNotBlockResubscribe(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	deleted := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, trackedRepository.ID)

	err = subscriptionStore.CancelByCancellationToken(ctx, deleted.CancellationToken.String())
	require.NoError(t, err)

	created := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, trackedRepository.ID)
	require.NotEqual(t, deleted.ID, created.ID)
	requireSubscribeSagaStatus(t, db, created.ID, subscriptions.SagaStatusCompleted)
}

func TestSubscriptionStore_DeletedPendingSubscriptionDoesNotBlockResubscribe(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)

	pending, err := subscriptionStore.CreatePending(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	err = subscriptionStore.DeleteByID(ctx, pending.ID)
	require.NoError(t, err)

	created := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, trackedRepository.ID)
	require.NotEqual(t, pending.ID, created.ID)
	requireSubscribeSagaStatus(t, db, created.ID, subscriptions.SagaStatusCompleted)
}

func TestSubscriptionStore_ListByEmail(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	firstRepoData := test.NewTestRepositoryWithPrefix("a")
	secondRepoData := test.NewTestRepositoryWithPrefix("z")

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	firstRepo, err := trackedRepositoryStore.CreateIfNotExists(ctx, firstRepoData.Owner, firstRepoData.Name)
	require.NoError(t, err)
	err = trackedRepositoryStore.UpdateLastSeenTag(ctx, firstRepo.ID, "v1.11.0")
	require.NoError(t, err)

	secondRepo, err := trackedRepositoryStore.CreateIfNotExists(ctx, secondRepoData.Owner, secondRepoData.Name)
	require.NoError(t, err)
	err = trackedRepositoryStore.UpdateLastSeenTag(ctx, secondRepo.ID, "v4.13.4")
	require.NoError(t, err)

	firstSubscription := createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, firstRepo.ID)

	_ = createCompletedSubscription(t, db, subscriptionStore, ctx, user.ID, secondRepo.ID)

	err = subscriptionStore.ConfirmByToken(ctx, firstSubscription.ConfirmationToken.String())
	require.NoError(t, err)

	items, err := subscriptionStore.ListByEmail(ctx, email)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, subscriptions.SubscriptionView{
		Email:       email,
		Repo:        firstRepoData.Owner + "/" + firstRepoData.Name,
		Confirmed:   true,
		LastSeenTag: "v1.11.0",
	}, items[0])
}

func TestSubscriptionStore_ListByEmail_Empty(t *testing.T) {
	db := test.SetupTestDB(t)
	subscriptionStore := NewSubscriptionStore(db)

	items, err := subscriptionStore.ListByEmail(context.Background(), test.NewTestEmail())
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestSubscriptionStore_Create_ReturnsForeignKeyErrorForUnknownReferences(t *testing.T) {
	db := test.SetupTestDB(t)
	subscriptionStore := NewSubscriptionStore(db)

	_, err := subscriptionStore.CreatePending(context.Background(), 999, 999)
	require.Error(t, err)
	require.False(t, errors.Is(err, subscriptions.ErrAlreadyExists))
}

func createCompletedSubscription(t *testing.T, db *sql.DB, store *SubscriptionStore, ctx context.Context, userID int64, trackedRepositoryID int64) subscriptions.Subscription {
	t.Helper()

	created, err := store.CreatePending(ctx, userID, trackedRepositoryID)
	require.NoError(t, err)

	_, err = db.ExecContext(
		ctx,
		`
			insert into subscription_sagas (id, subscription_id, operation, status)
			values ($1, $2, $3, $4);
		`,
		uuid.New(),
		created.ID,
		subscriptions.SagaOperationSubscribe,
		subscriptions.SagaStatusCompleted,
	)
	require.NoError(t, err)

	return created
}

func requireSubscribeSagaStatus(t *testing.T, db *sql.DB, subscriptionID int64, expected subscriptions.SagaStatus) {
	t.Helper()

	var actual subscriptions.SagaStatus
	err := db.QueryRowContext(
		context.Background(),
		`select status from subscription_sagas where subscription_id = $1 and operation = 'subscribe'`,
		subscriptionID,
	).Scan(&actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func requireNoSubscribeSaga(t *testing.T, db *sql.DB, subscriptionID int64) {
	t.Helper()

	var count int
	err := db.QueryRowContext(
		context.Background(),
		`select count(*) from subscription_sagas where subscription_id = $1 and operation = 'subscribe'`,
		subscriptionID,
	).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireSubscriptionMissing(t *testing.T, db *sql.DB, subscriptionID int64) {
	t.Helper()

	var count int
	err := db.QueryRowContext(context.Background(), `select count(*) from subscriptions where id = $1`, subscriptionID).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count)
}
