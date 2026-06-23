package subscriptions

import (
	"context"
	"errors"

	releasetracking "github-release-notifier/internal/app/release_tracking"

	"github.com/google/uuid"
)

type txManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type userStore interface {
	CreateIfNotExists(ctx context.Context, email string) (User, error)
}

type trackedRepositoryCreator interface {
	CreateIfNotExists(ctx context.Context, owner string, name string) (releasetracking.TrackedRepository, error)
}

type subscriptionStore interface {
	CreatePending(ctx context.Context, userID int64, trackedRepositoryID int64) (Subscription, error)
	ConfirmByToken(ctx context.Context, confirmationToken string) error
	ListByEmail(ctx context.Context, email string) ([]SubscriptionView, error)
}

type subscriptionCancellationReader interface {
	FindByCancellationToken(ctx context.Context, cancellationToken string) (Subscription, error)
}

type subscriptionSagaWriter interface {
	Create(ctx context.Context, saga Saga) error
}

type subscriptionSagaOrchestrator interface {
	ProcessByID(ctx context.Context, sagaID uuid.UUID) error
}

type confirmationQueue interface {
	QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error
}

type githubRepositoryClient interface {
	RepositoryExists(ctx context.Context, owner string, repoName string) error
}

type Service struct {
	txManager     txManager
	users         userStore
	repositories  trackedRepositoryCreator
	subscriptions subscriptionStore
	sagaReader    subscriptionCancellationReader
	sagas         subscriptionSagaWriter
	orchestrator  subscriptionSagaOrchestrator
	repositoryAPI githubRepositoryClient
}

func NewService(
	txManager txManager,
	users userStore,
	repositories trackedRepositoryCreator,
	subscriptions subscriptionStore,
	sagaReader subscriptionCancellationReader,
	sagas subscriptionSagaWriter,
	orchestrator subscriptionSagaOrchestrator,
	repositoryAPI githubRepositoryClient,
) *Service {
	return &Service{
		txManager:     txManager,
		users:         users,
		repositories:  repositories,
		subscriptions: subscriptions,
		sagaReader:    sagaReader,
		sagas:         sagas,
		orchestrator:  orchestrator,
		repositoryAPI: repositoryAPI,
	}
}

func (s *Service) Subscribe(ctx context.Context, email string, repositoryFullName string) error {
	repository, err := s.prepareSubscription(ctx, repositoryFullName)
	if err != nil {
		return err
	}

	return s.subscribeWithSaga(ctx, email, repository)
}

func (s *Service) subscribeWithSaga(ctx context.Context, email string, repository subscriptionRepository) error {
	sagaID := uuid.New()

	if err := s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.users.CreateIfNotExists(ctx, email)
		if err != nil {
			return err
		}

		trackedRepository, err := s.repositories.CreateIfNotExists(ctx, repository.owner, repository.name)
		if err != nil {
			return err
		}

		subscription, err := s.subscriptions.CreatePending(ctx, user.ID, trackedRepository.ID)
		if err != nil {
			return err
		}

		return s.sagas.Create(ctx, Saga{
			ID:             sagaID,
			SubscriptionID: subscription.ID,
			Operation:      SagaOperationSubscribe,
		})
	}); err != nil {
		return err
	}

	return s.orchestrator.ProcessByID(ctx, sagaID)
}

func (s *Service) ConfirmSubscription(ctx context.Context, token string) error {
	return s.subscriptions.ConfirmByToken(ctx, token)
}

func (s *Service) CancelSubscription(ctx context.Context, token string) error {
	return s.cancelWithSaga(ctx, token)
}

func (s *Service) cancelWithSaga(ctx context.Context, token string) error {
	sagaID := uuid.New()

	sagaCreated := true
	if err := s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		subscription, err := s.sagaReader.FindByCancellationToken(ctx, token)
		if err != nil {
			return err
		}

		if err := s.sagas.Create(ctx, Saga{
			ID:             sagaID,
			SubscriptionID: subscription.ID,
			Operation:      SagaOperationUnsubscribe,
		}); err != nil {
			if errors.Is(err, ErrAlreadyExists) {
				sagaCreated = false
				return nil
			}

			return err
		}

		return nil
	}); err != nil {
		return err
	}

	if !sagaCreated {
		return nil
	}

	return s.orchestrator.ProcessByID(ctx, sagaID)
}

func (s *Service) ListSubscriptions(ctx context.Context, email string) ([]SubscriptionView, error) {
	return s.subscriptions.ListByEmail(ctx, email)
}
