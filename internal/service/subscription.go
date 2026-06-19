package service

import (
	"context"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/readmodel"

	"github.com/google/uuid"
)

type txManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type UserStore interface {
	CreateIfNotExists(ctx context.Context, email string) (domain.User, error)
}

type trackedRepositoryCreator interface {
	CreateIfNotExists(ctx context.Context, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error)
}

type SubscriptionStore interface {
	Create(ctx context.Context, userID int64, trackedRepositoryID int64) (domain.Subscription, error)
	SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error
	DeleteByCancellationToken(ctx context.Context, cancellationToken string) error
	ListByEmail(ctx context.Context, email string) ([]readmodel.SubscriptionView, error)
}

type ConfirmationQueue interface {
	QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error
}

type SubscriptionService struct {
	txManager     txManager
	users         UserStore
	repositories  trackedRepositoryCreator
	subscriptions SubscriptionStore
	repositoryAPI githubRepositoryClient
	mailQueue     ConfirmationQueue
}

func NewSubscriptionService(
	txManager txManager,
	users UserStore,
	repositories trackedRepositoryCreator,
	subscriptions SubscriptionStore,
	repositoryAPI githubRepositoryClient,
	mailQueue ConfirmationQueue,
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

func (s *SubscriptionService) ConfirmSubscription(ctx context.Context, token string) error {
	return s.subscriptions.SetConfirmedByTokenAndConfirmedNotTrue(ctx, token)
}

func (s *SubscriptionService) CancelSubscription(ctx context.Context, token string) error {
	return s.subscriptions.DeleteByCancellationToken(ctx, token)
}

func (s *SubscriptionService) ListSubscriptions(ctx context.Context, email string) ([]readmodel.SubscriptionView, error) {
	return s.subscriptions.ListByEmail(ctx, email)
}
