package service

import (
	"context"
	"database/sql"

	"github-release-notifier/internal/domain"
	ghclient "github-release-notifier/internal/github"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/storage"
)

type userStoreAdapter struct {
	store *storage.UserStore
}

func (a userStoreAdapter) CreateIfNotExists(ctx context.Context, email string) (domain.User, error) {
	return a.store.CreateIfNotExists(ctx, email)
}

func (a userStoreAdapter) WithTx(tx *sql.Tx) userCreator {
	return userStoreAdapter{store: a.store.WithTx(tx)}
}

type trackedRepositoryStoreAdapter struct {
	store *storage.TrackedRepositoryStore
}

func (a trackedRepositoryStoreAdapter) CreatIfNotExists(ctx context.Context, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error) {
	return a.store.CreatIfNotExists(ctx, owner, name, lastSeenTag)
}

func (a trackedRepositoryStoreAdapter) WithTx(tx *sql.Tx) trackedRepositoryProvider {
	return trackedRepositoryStoreAdapter{store: a.store.WithTx(tx)}
}

type subscriptionStoreAdapter struct {
	store *storage.SubscriptionStore
}

func (a subscriptionStoreAdapter) Create(ctx context.Context, userID int64, trackedRepositoryID int64) (domain.Subscription, error) {
	return a.store.Create(ctx, userID, trackedRepositoryID)
}

func (a subscriptionStoreAdapter) SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error {
	return a.store.SetConfirmedByTokenAndConfirmedNotTrue(ctx, confirmationToken)
}

func (a subscriptionStoreAdapter) DeleteByCancellationToken(ctx context.Context, cancellationToken string) error {
	return a.store.DeleteByCancellationToken(ctx, cancellationToken)
}

func (a subscriptionStoreAdapter) WithTx(tx *sql.Tx) subscriptionCreator {
	return subscriptionStoreAdapter{store: a.store.WithTx(tx)}
}

type githubClientAdapter struct {
	client *ghclient.Client
}

func (a githubClientAdapter) GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error) {
	return a.client.GetLatestRelease(ctx, owner, repoName)
}

func NewSubscriptionServiceFromStorage(
	transactionManager transactionManager,
	users *storage.UserStore,
	trackedRepositories *storage.TrackedRepositoryStore,
	subscriptions *storage.SubscriptionStore,
	githubClient *ghclient.Client,
	confirmationSender *mail.Service,
) *SubscriptionService {
	return NewSubscriptionService(
		transactionManager,
		userStoreAdapter{store: users},
		trackedRepositoryStoreAdapter{store: trackedRepositories},
		subscriptionStoreAdapter{store: subscriptions},
		githubClientAdapter{client: githubClient},
		confirmationSender,
	)
}
