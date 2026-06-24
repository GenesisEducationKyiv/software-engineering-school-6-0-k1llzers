//go:build integration

package repository

import (
	"context"
	"github-release-notifier/internal/platform/db/test"
	"testing"

	releasetracking "github-release-notifier/internal/app/release_tracking"
	subscriptionsrepo "github-release-notifier/internal/app/subscriptions/repository"

	"github.com/stretchr/testify/require"
)

func TestConfirmedSubscriptionStore_ListConfirmedRepositorySubscriptions_ReturnsOnlyConfirmedSubscriptions(t *testing.T) {
	db := test.SetupTestDB(t)

	userStore := subscriptionsrepo.NewUserStore(db)
	trackedRepositoryStore := NewTrackedRepositoryStore(db)
	subscriptionStore := subscriptionsrepo.NewSubscriptionStore(db)
	confirmedSubscriptionStore := NewConfirmedSubscriptionStore(db)

	ctx := context.Background()
	firstUserEmail := test.NewTestEmail()
	secondUserEmail := test.NewTestEmail()
	repositoryData := test.NewTestRepository()
	firstUser, err := userStore.CreateIfNotExists(ctx, firstUserEmail)
	require.NoError(t, err)

	secondUser, err := userStore.CreateIfNotExists(ctx, secondUserEmail)
	require.NoError(t, err)

	repository, err := trackedRepositoryStore.CreateIfNotExists(ctx, repositoryData.Owner, repositoryData.Name)
	require.NoError(t, err)
	err = trackedRepositoryStore.UpdateLastSeenTag(ctx, repository.ID, "v1.11.0")
	require.NoError(t, err)

	firstSubscription, err := subscriptionStore.CreatePending(ctx, firstUser.ID, repository.ID)
	require.NoError(t, err)

	secondSubscription, err := subscriptionStore.CreatePending(ctx, secondUser.ID, repository.ID)
	require.NoError(t, err)

	err = subscriptionStore.SetConfirmedByTokenAndConfirmedNotTrue(ctx, firstSubscription.ConfirmationToken.String())
	require.NoError(t, err)

	items, err := confirmedSubscriptionStore.ListConfirmedRepositorySubscriptions(ctx)
	require.NoError(t, err)
	expected := releasetracking.ConfirmedRepositorySubscription{
		TrackedRepositoryID: repository.ID,
		Owner:               repositoryData.Owner,
		Name:                repositoryData.Name,
		LastSeenTag:         strPtr("v1.11.0"),
		Email:               firstUserEmail,
		CancellationToken:   firstSubscription.CancellationToken,
	}
	require.Contains(t, items, expected)
	for _, item := range items {
		require.NotEqual(t, secondUserEmail, item.Email)
	}
	require.NotEqual(t, firstSubscription.CancellationToken, secondSubscription.CancellationToken)
}

func strPtr(value string) *string {
	return &value
}
