//go:build integration

package repository

import (
	"context"
	"errors"
	"github-release-notifier/internal/platform/db/test"
	"testing"

	dbtx "github-release-notifier/internal/platform/db"
	releasetrackingrepo "github-release-notifier/internal/release_tracking/repository"
	"github-release-notifier/internal/shared"
	"github-release-notifier/internal/subscriptions"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionStore_Create(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name, "v1.0.0")
	require.NoError(t, err)

	created, err := subscriptionStore.Create(ctx, user.ID, trackedRepository.ID)
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

	repository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name, "")
	require.NoError(t, err)

	tx := test.BeginTestTx(t, db)
	created, err := subscriptionStore.Create(dbtx.WithTransactionContext(ctx, tx), user.ID, repository.ID)
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

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name, "v1.0.0")
	require.NoError(t, err)

	_, err = subscriptionStore.Create(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	_, err = subscriptionStore.Create(ctx, user.ID, trackedRepository.ID)
	require.ErrorIs(t, err, subscriptions.ErrAlreadyExists)
}

func TestSubscriptionStore_SetConfirmedByTokenAndConfirmedNotTrue(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name, "v1.0.0")
	require.NoError(t, err)

	created, err := subscriptionStore.Create(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	err = subscriptionStore.SetConfirmedByTokenAndConfirmedNotTrue(ctx, created.ConfirmationToken.String())
	require.NoError(t, err)

	var confirmed bool
	err = db.QueryRowContext(ctx, `select confirmed from subscriptions where id = $1`, created.ID).Scan(&confirmed)
	require.NoError(t, err)
	require.True(t, confirmed)
}

func TestSubscriptionStore_SetConfirmedByTokenAndConfirmedNotTrue_ReturnsInvalidTokenWhenAlreadyConfirmed(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name, "v1.0.0")
	require.NoError(t, err)

	created, err := subscriptionStore.Create(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	err = subscriptionStore.SetConfirmedByTokenAndConfirmedNotTrue(ctx, created.ConfirmationToken.String())
	require.NoError(t, err)

	err = subscriptionStore.SetConfirmedByTokenAndConfirmedNotTrue(ctx, created.ConfirmationToken.String())
	require.ErrorIs(t, err, subscriptions.ErrInvalidToken)
}

func TestSubscriptionStore_SetConfirmedByTokenAndConfirmedNotTrue_NotFound(t *testing.T) {
	db := test.SetupTestDB(t)
	subscriptionStore := NewSubscriptionStore(db)

	err := subscriptionStore.SetConfirmedByTokenAndConfirmedNotTrue(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestSubscriptionStore_DeleteByCancellationToken(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := NewUserStore(db)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(db)
	subscriptionStore := NewSubscriptionStore(db)

	ctx := context.Background()
	email := test.NewTestEmail()
	repositoryData := test.NewTestRepository()

	user, err := userStore.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	trackedRepository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name, "v1.0.0")
	require.NoError(t, err)

	created, err := subscriptionStore.Create(ctx, user.ID, trackedRepository.ID)
	require.NoError(t, err)

	err = subscriptionStore.DeleteByCancellationToken(ctx, created.CancellationToken.String())
	require.NoError(t, err)

	var count int
	err = db.QueryRowContext(ctx, `select count(*) from subscriptions where id = $1`, created.ID).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestSubscriptionStore_DeleteByCancellationToken_NotFound(t *testing.T) {
	db := test.SetupTestDB(t)
	subscriptionStore := NewSubscriptionStore(db)

	err := subscriptionStore.DeleteByCancellationToken(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, shared.ErrNotFound)
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

	firstRepo, err := trackedRepositoryStore.CreateIfNotExists(ctx, firstRepoData.Owner, firstRepoData.Name, "v1.11.0")
	require.NoError(t, err)

	secondRepo, err := trackedRepositoryStore.CreateIfNotExists(ctx, secondRepoData.Owner, secondRepoData.Name, "v4.13.4")
	require.NoError(t, err)

	firstSubscription, err := subscriptionStore.Create(ctx, user.ID, firstRepo.ID)
	require.NoError(t, err)

	_, err = subscriptionStore.Create(ctx, user.ID, secondRepo.ID)
	require.NoError(t, err)

	err = subscriptionStore.SetConfirmedByTokenAndConfirmedNotTrue(ctx, firstSubscription.ConfirmationToken.String())
	require.NoError(t, err)

	items, err := subscriptionStore.ListByEmail(ctx, email)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, subscriptions.SubscriptionView{
		Email:       email,
		Repo:        firstRepoData.Owner + "/" + firstRepoData.Name,
		Confirmed:   true,
		LastSeenTag: "v1.11.0",
	}, items[0])
	require.Equal(t, subscriptions.SubscriptionView{
		Email:       email,
		Repo:        secondRepoData.Owner + "/" + secondRepoData.Name,
		Confirmed:   false,
		LastSeenTag: "v4.13.4",
	}, items[1])
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

	_, err := subscriptionStore.Create(context.Background(), 999, 999)
	require.Error(t, err)
	require.False(t, errors.Is(err, subscriptions.ErrAlreadyExists))
}
