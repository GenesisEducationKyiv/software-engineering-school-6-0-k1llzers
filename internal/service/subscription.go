package service

import (
	"context"
	"database/sql"
	"strings"

	"github-release-notifier/internal/domain"

	"github.com/google/uuid"
)

type transactionManager interface {
	WithinTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
}

type userCreator interface {
	CreateIfNotExists(ctx context.Context, email string) (domain.User, error)
	WithTx(tx *sql.Tx) userCreator
}

type trackedRepositoryProvider interface {
	CreatIfNotExists(ctx context.Context, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error)
	WithTx(tx *sql.Tx) trackedRepositoryProvider
}

type subscriptionCreator interface {
	Create(ctx context.Context, userID int64, trackedRepositoryID int64) (domain.Subscription, error)
	SetConfirmedByTokenAndConfirmedNotTrue(ctx context.Context, confirmationToken string) error
	DeleteByCancellationToken(ctx context.Context, cancellationToken string) error
	WithTx(tx *sql.Tx) subscriptionCreator
}

type gitRepositoryProvider interface {
	GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error)
}

type subscriptionConfirmationSender interface {
	SendSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error
}

type SubscriptionService struct {
	transactionManager    transactionManager
	users                 userCreator
	trackedRepositories   trackedRepositoryProvider
	subscriptions         subscriptionCreator
	gitRepositoryProvider gitRepositoryProvider
	confirmationSender    subscriptionConfirmationSender
}

func NewSubscriptionService(
	transactionManager transactionManager,
	users userCreator,
	trackedRepositories trackedRepositoryProvider,
	subscriptions subscriptionCreator,
	gitRepositoryProvider gitRepositoryProvider,
	confirmationSender subscriptionConfirmationSender,
) *SubscriptionService {
	return &SubscriptionService{
		transactionManager:    transactionManager,
		users:                 users,
		trackedRepositories:   trackedRepositories,
		subscriptions:         subscriptions,
		gitRepositoryProvider: gitRepositoryProvider,
		confirmationSender:    confirmationSender,
	}
}

func (s *SubscriptionService) Subscribe(ctx context.Context, email string, repositoryFullName string) error {
	owner, repoName, err := splitRepositoryFullName(repositoryFullName)
	if err != nil {
		return err
	}

	repository, err := s.gitRepositoryProvider.GetLatestRelease(ctx, owner, repoName)
	if err != nil {
		return err
	}

	var subscription domain.Subscription

	err = s.transactionManager.WithinTransaction(ctx, func(tx *sql.Tx) error {
		users := s.users.WithTx(tx)
		trackedRepositories := s.trackedRepositories.WithTx(tx)
		subscriptions := s.subscriptions.WithTx(tx)

		user, err := users.CreateIfNotExists(ctx, email)
		if err != nil {
			return err
		}

		trackedRepository, err := trackedRepositories.CreatIfNotExists(ctx, owner, repoName, repository.TagName)
		if err != nil {
			return err
		}

		subscription, err = subscriptions.Create(ctx, user.ID, trackedRepository.ID)
		if err != nil {
			return err
		}

		if err := s.confirmationSender.SendSubscriptionConfirmation(ctx, email, repositoryFullName, subscription.ConfirmationToken, subscription.CancellationToken); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *SubscriptionService) ConfirmSubscription(ctx context.Context, token string) error {
	return s.subscriptions.SetConfirmedByTokenAndConfirmedNotTrue(ctx, token)
}

func (s *SubscriptionService) CancelSubscription(ctx context.Context, token string) error {
	return s.subscriptions.DeleteByCancellationToken(ctx, token)
}

func splitRepositoryFullName(repositoryFullName string) (string, string, error) {
	parts := strings.Split(repositoryFullName, "/")

	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", domain.ErrIncorrectRepositoryFormat
	}
	return parts[0], parts[1], nil
}
