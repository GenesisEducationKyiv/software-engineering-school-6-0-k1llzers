package service

import (
	"context"
	"database/sql"
	"strings"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/readmodel"
)

type txManager interface {
	WithinTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
}

type UserStore interface {
	CreateIfNotExists(ctx context.Context, email string) (domain.User, error)
}

type TrackedRepositoryStore interface {
	CreatIfNotExists(ctx context.Context, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error)
	UpdateLastSeenTag(ctx context.Context, trackedRepositoryID int64, lastSeenTag string) error
}

type SubscriptionStore interface {
	Create(ctx context.Context, userID int64, trackedRepositoryID int64) (domain.Subscription, error)
	SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error
	DeleteByCancellationToken(ctx context.Context, cancellationToken string) error
	ListByEmail(ctx context.Context, email string) ([]readmodel.SubscriptionView, error)
	ListConfirmedRepositorySubscriptions(ctx context.Context) ([]readmodel.ConfirmedRepositorySubscription, error)
}

type githubReleaseClient interface {
	GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error)
}

type TxUserStoreFactory func(tx *sql.Tx) UserStore
type TxTrackedRepositoryStoreFactory func(tx *sql.Tx) TrackedRepositoryStore
type TxSubscriptionStoreFactory func(tx *sql.Tx) SubscriptionStore

type SubscriptionService struct {
	txManager              txManager
	usersWithTx            TxUserStoreFactory
	repositoriesWithTx     TxTrackedRepositoryStoreFactory
	subscriptions          SubscriptionStore
	subscriptionsWithTx    TxSubscriptionStoreFactory
	releaseClient          githubReleaseClient
	transactionalMailQueue mail.QueueFactory
}

func NewSubscriptionService(
	txManager txManager,
	usersWithTx TxUserStoreFactory,
	repositoriesWithTx TxTrackedRepositoryStoreFactory,
	subscriptions SubscriptionStore,
	subscriptionsWithTx TxSubscriptionStoreFactory,
	releaseClient githubReleaseClient,
	transactionalMailQueue mail.QueueFactory,
) *SubscriptionService {
	return &SubscriptionService{
		txManager:              txManager,
		usersWithTx:            usersWithTx,
		repositoriesWithTx:     repositoriesWithTx,
		subscriptions:          subscriptions,
		subscriptionsWithTx:    subscriptionsWithTx,
		releaseClient:          releaseClient,
		transactionalMailQueue: transactionalMailQueue,
	}
}

func (s *SubscriptionService) Subscribe(ctx context.Context, email string, repositoryFullName string) error {
	owner, repoName, err := splitRepositoryFullName(repositoryFullName)
	if err != nil {
		return err
	}

	release, err := s.releaseClient.GetLatestRelease(ctx, owner, repoName)
	if err != nil {
		return err
	}

	return s.txManager.WithinTransaction(ctx, func(tx *sql.Tx) error {
		users := s.usersWithTx(tx)
		repositories := s.repositoriesWithTx(tx)
		subscriptions := s.subscriptionsWithTx(tx)
		notifications := s.transactionalMailQueue.WithTx(tx)

		user, err := users.CreateIfNotExists(ctx, email)
		if err != nil {
			return err
		}

		repository, err := repositories.CreatIfNotExists(ctx, owner, repoName, release.TagName)
		if err != nil {
			return err
		}

		subscription, err := subscriptions.Create(ctx, user.ID, repository.ID)
		if err != nil {
			return err
		}

		return notifications.QueueSubscriptionConfirmation(ctx, email, repositoryFullName, subscription.ConfirmationToken, subscription.CancellationToken)
	})
}

func (s *SubscriptionService) ConfirmSubscription(ctx context.Context, token string) error {
	return s.subscriptions.SetConfirmedByTokenAndConfirmedNotTrue(ctx, token)
}

func (s *SubscriptionService) CancelSubscription(ctx context.Context, token string) error {
	return s.subscriptions.DeleteByCancellationToken(ctx, token)
}

func (s *SubscriptionService) ListSubscriptions(ctx context.Context, email string) ([]readmodel.SubscriptionView, error) {
	return s.subscriptions.ListByEmail(ctx, email)
}

func splitRepositoryFullName(repositoryFullName string) (string, string, error) {
	parts := strings.Split(repositoryFullName, "/")

	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", domain.ErrIncorrectRepositoryFormat
	}
	return parts[0], parts[1], nil
}
