package subscriptions

import (
	"context"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/readmodel"

	"github.com/google/uuid"
)

type txManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type userStore interface {
	CreateIfNotExists(ctx context.Context, email string) (domain.User, error)
}

type trackedRepositoryCreator interface {
	CreateIfNotExists(ctx context.Context, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error)
}

type subscriptionStore interface {
	Create(ctx context.Context, userID int64, trackedRepositoryID int64) (domain.Subscription, error)
	SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error
	DeleteByCancellationToken(ctx context.Context, cancellationToken string) error
	ListByEmail(ctx context.Context, email string) ([]readmodel.SubscriptionView, error)
}

type confirmationQueue interface {
	QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error
}

type githubRepositoryClient interface {
	RepositoryExists(ctx context.Context, owner string, repoName string) error
	GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error)
}

type Service struct {
	txManager     txManager
	users         userStore
	repositories  trackedRepositoryCreator
	subscriptions subscriptionStore
	repositoryAPI githubRepositoryClient
	mailQueue     confirmationQueue
}

func NewService(
	txManager txManager,
	users userStore,
	repositories trackedRepositoryCreator,
	subscriptions subscriptionStore,
	repositoryAPI githubRepositoryClient,
	mailQueue confirmationQueue,
) *Service {
	return &Service{
		txManager:     txManager,
		users:         users,
		repositories:  repositories,
		subscriptions: subscriptions,
		repositoryAPI: repositoryAPI,
		mailQueue:     mailQueue,
	}
}

func (s *Service) Subscribe(ctx context.Context, email string, repositoryFullName string) error {
	repository, err := s.prepareSubscription(ctx, repositoryFullName)
	if err != nil {
		return err
	}

	return s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.users.CreateIfNotExists(ctx, email)
		if err != nil {
			return err
		}

		trackedRepository, err := s.repositories.CreateIfNotExists(ctx, repository.owner, repository.name, repository.lastSeenTag)
		if err != nil {
			return err
		}

		subscription, err := s.subscriptions.Create(ctx, user.ID, trackedRepository.ID)
		if err != nil {
			return err
		}

		return s.mailQueue.QueueSubscriptionConfirmation(ctx, email, repositoryFullName, subscription.ConfirmationToken, subscription.CancellationToken)
	})
}

func (s *Service) ConfirmSubscription(ctx context.Context, token string) error {
	return s.subscriptions.SetConfirmedByTokenAndConfirmedNotTrue(ctx, token)
}

func (s *Service) CancelSubscription(ctx context.Context, token string) error {
	return s.subscriptions.DeleteByCancellationToken(ctx, token)
}

func (s *Service) ListSubscriptions(ctx context.Context, email string) ([]readmodel.SubscriptionView, error) {
	return s.subscriptions.ListByEmail(ctx, email)
}
