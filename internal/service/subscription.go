package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/readmodel"
)

type txManager interface {
	WithinTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
}

type UserStore interface {
	CreateIfNotExists(ctx context.Context, tx *sql.Tx, email string) (domain.User, error)
}

type TrackedRepositoryStore interface {
	CreateIfNotExists(ctx context.Context, tx *sql.Tx, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error)
	UpdateLastSeenTag(ctx context.Context, tx *sql.Tx, trackedRepositoryID int64, lastSeenTag string) error
}

type SubscriptionStore interface {
	Create(ctx context.Context, tx *sql.Tx, userID int64, trackedRepositoryID int64) (domain.Subscription, error)
	SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error
	DeleteByCancellationToken(ctx context.Context, cancellationToken string) error
	ListByEmail(ctx context.Context, email string) ([]readmodel.SubscriptionView, error)
	ListConfirmedRepositorySubscriptions(ctx context.Context) ([]readmodel.ConfirmedRepositorySubscription, error)
}

type githubRepositoryClient interface {
	RepositoryExists(ctx context.Context, owner string, repoName string) error
	GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error)
}

type SubscriptionService struct {
	txManager     txManager
	users         UserStore
	repositories  TrackedRepositoryStore
	subscriptions SubscriptionStore
	repositoryAPI githubRepositoryClient
	mailQueue     mail.Queue
}

func NewSubscriptionService(
	txManager txManager,
	users UserStore,
	repositories TrackedRepositoryStore,
	subscriptions SubscriptionStore,
	repositoryAPI githubRepositoryClient,
	mailQueue mail.Queue,
) *SubscriptionService {
	return &SubscriptionService{
		txManager:     txManager,
		users:         users,
		repositories:  repositories,
		subscriptions: subscriptions,
		repositoryAPI: repositoryAPI,
		mailQueue:     mailQueue,
	}
}

func (s *SubscriptionService) Subscribe(ctx context.Context, email string, repositoryFullName string) error {
	owner, repoName, err := splitRepositoryFullName(repositoryFullName)
	if err != nil {
		return err
	}

	if err := s.repositoryAPI.RepositoryExists(ctx, owner, repoName); err != nil {
		return err
	}

	release, err := s.repositoryAPI.GetLatestRelease(ctx, owner, repoName)
	if err != nil && !errors.Is(err, domain.ErrNoReleases) {
		return err
	}

	lastSeenTag := ""
	if err == nil {
		lastSeenTag = release.TagName
	}

	return s.txManager.WithinTransaction(ctx, func(tx *sql.Tx) error {
		user, err := s.users.CreateIfNotExists(ctx, tx, email)
		if err != nil {
			return err
		}

		repository, err := s.repositories.CreateIfNotExists(ctx, tx, owner, repoName, lastSeenTag)
		if err != nil {
			return err
		}

		subscription, err := s.subscriptions.Create(ctx, tx, user.ID, repository.ID)
		if err != nil {
			return err
		}

		return s.mailQueue.QueueSubscriptionConfirmation(ctx, tx, email, repositoryFullName, subscription.ConfirmationToken, subscription.CancellationToken)
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
