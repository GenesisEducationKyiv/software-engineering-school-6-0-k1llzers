//go:build unit

package subscriptions

import (
	"context"
	"errors"
	"testing"

	"github-release-notifier/internal/domain"
	releasetracking "github-release-notifier/internal/release_tracking"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type userCreatorStub struct {
	user  User
	err   error
	email string
}

func (s *userCreatorStub) CreateIfNotExists(_ context.Context, email string) (User, error) {
	s.email = email
	if s.err != nil {
		return User{}, s.err
	}

	return s.user, nil
}

type trackedRepositoryProviderStub struct {
	result      releasetracking.TrackedRepository
	err         error
	owner       string
	repoName    string
	lastSeenTag string
	updatedID   int64
	updatedTag  string
}

func (s *trackedRepositoryProviderStub) CreateIfNotExists(_ context.Context, owner string, name string, lastSeenTag string) (releasetracking.TrackedRepository, error) {
	s.owner = owner
	s.repoName = name
	s.lastSeenTag = lastSeenTag
	if s.err != nil {
		return releasetracking.TrackedRepository{}, s.err
	}

	return s.result, nil
}

func (s *trackedRepositoryProviderStub) UpdateLastSeenTag(_ context.Context, trackedRepositoryID int64, lastSeenTag string) error {
	s.updatedID = trackedRepositoryID
	s.updatedTag = lastSeenTag
	return s.err
}

type gitRepositoryProviderStub struct {
	result     releasetracking.Release
	releaseErr error
	existsErr  error
	owner      string
	repoName   string
	calls      int
}

func (s *gitRepositoryProviderStub) RepositoryExists(_ context.Context, owner string, repoName string) error {
	s.owner = owner
	s.repoName = repoName
	return s.existsErr
}

func (s *gitRepositoryProviderStub) GetLatestRelease(_ context.Context, owner string, repoName string) (releasetracking.Release, error) {
	s.calls++
	s.owner = owner
	s.repoName = repoName
	if s.releaseErr != nil {
		return releasetracking.Release{}, s.releaseErr
	}

	return s.result, nil
}

type subscriptionCreatorStub struct {
	result              Subscription
	listResult          []SubscriptionView
	err                 error
	createdUserID       int64
	createdRepositoryID int64
	confirmedToken      string
	cancellationToken   string
	listEmail           string
}

func (s *subscriptionCreatorStub) Create(_ context.Context, userID int64, trackedRepositoryID int64) (Subscription, error) {
	s.createdUserID = userID
	s.createdRepositoryID = trackedRepositoryID
	if s.err != nil {
		return Subscription{}, s.err
	}

	return s.result, nil
}

func (s *subscriptionCreatorStub) SetConfirmedByTokenAndConfirmedNotTrue(_ context.Context, confirmationToken string) error {
	s.confirmedToken = confirmationToken
	if s.err != nil {
		return s.err
	}

	return nil
}

func (s *subscriptionCreatorStub) DeleteByCancellationToken(_ context.Context, cancellationToken string) error {
	s.cancellationToken = cancellationToken
	if s.err != nil {
		return s.err
	}

	return nil
}

func (s *subscriptionCreatorStub) ListByEmail(_ context.Context, email string) ([]SubscriptionView, error) {
	s.listEmail = email
	if s.err != nil {
		return nil, s.err
	}

	return s.listResult, nil
}

type transactionManagerStub struct {
	err        error
	called     bool
	committed  bool
	rolledBack bool
}

func (s *transactionManagerStub) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	s.called = true
	err := fn(ctx)
	if err != nil {
		s.rolledBack = true
		return err
	}

	if s.err != nil {
		s.rolledBack = true
		return s.err
	}

	s.committed = true
	return nil
}

type confirmationSenderStub struct {
	err                error
	recipientEmail     string
	repositoryFullName string
	confirmationToken  uuid.UUID
	cancellationToken  uuid.UUID
	releaseRecipient   string
	releaseRepo        string
	releaseTag         string
	releaseURL         string
	called             bool
}

func (s *confirmationSenderStub) SendSubscriptionConfirmation(_ context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	s.called = true
	s.recipientEmail = recipientEmail
	s.repositoryFullName = repositoryFullName
	s.confirmationToken = confirmationToken
	s.cancellationToken = cancellationToken
	return s.err
}

func (s *confirmationSenderStub) QueueSubscriptionConfirmation(ctx context.Context, recipientEmail string, repositoryFullName string, confirmationToken uuid.UUID, cancellationToken uuid.UUID) error {
	return s.SendSubscriptionConfirmation(ctx, recipientEmail, repositoryFullName, confirmationToken, cancellationToken)
}

func (s *confirmationSenderStub) QueueReleaseNotification(_ context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error {
	s.called = true
	s.releaseRecipient = recipientEmail
	s.releaseRepo = repositoryFullName
	s.releaseTag = tagName
	s.releaseURL = releaseURL
	s.cancellationToken = cancellationToken
	return s.err
}

func TestSubscriptionService_Subscribe_UsesExistingTrackedRepository(t *testing.T) {
	users := &userCreatorStub{user: User{ID: 10, Email: "test@example.com"}}
	repositories := &trackedRepositoryProviderStub{result: releasetracking.TrackedRepository{ID: 20, Owner: "gin-gonic", Name: "gin"}}
	subscriptions := &subscriptionCreatorStub{
		result: Subscription{
			ID:                  30,
			UserID:              10,
			TrackedRepositoryID: 20,
			ConfirmationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	transactionManager := &transactionManagerStub{}
	releaseClient := &gitRepositoryProviderStub{result: releasetracking.Release{TagName: "v1.11.0"}}
	mailQueue := &confirmationSenderStub{}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, mailQueue)

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.NoError(t, err)
	require.Equal(t, "test@example.com", users.email)
	require.Equal(t, "gin-gonic", releaseClient.owner)
	require.Equal(t, "gin", releaseClient.repoName)
	require.Equal(t, int64(10), subscriptions.createdUserID)
	require.Equal(t, int64(20), subscriptions.createdRepositoryID)
	require.Equal(t, "gin-gonic", repositories.owner)
	require.Equal(t, "gin", repositories.repoName)
	require.Equal(t, "v1.11.0", repositories.lastSeenTag)
	require.True(t, mailQueue.called)
	require.Equal(t, "test@example.com", mailQueue.recipientEmail)
	require.Equal(t, "gin-gonic/gin", mailQueue.repositoryFullName)
	require.Equal(t, uuid.MustParse("11111111-1111-1111-1111-111111111111"), mailQueue.confirmationToken)
	require.Equal(t, uuid.MustParse("22222222-2222-2222-2222-222222222222"), mailQueue.cancellationToken)
	require.True(t, transactionManager.called)
	require.True(t, transactionManager.committed)
}

func TestSubscriptionService_Subscribe_ReturnsUserStoreError(t *testing.T) {
	expectedErr := errors.New("user store failed")
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{err: expectedErr}
	repositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{}
	releaseClient := &gitRepositoryProviderStub{result: releasetracking.Release{TagName: "v1.11.0"}}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.True(t, transactionManager.called)
	require.True(t, transactionManager.rolledBack)
}

func TestSubscriptionService_Subscribe_AllowsRepositoryWithoutReleases(t *testing.T) {
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{user: User{ID: 10, Email: "test@example.com"}}
	repositories := &trackedRepositoryProviderStub{result: releasetracking.TrackedRepository{ID: 20, Owner: "gin-gonic", Name: "gin"}}
	subscriptions := &subscriptionCreatorStub{
		result: Subscription{
			ID:                  30,
			UserID:              10,
			TrackedRepositoryID: 20,
			ConfirmationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	releaseClient := &gitRepositoryProviderStub{releaseErr: domain.ErrNoReleases}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.NoError(t, err)
	require.Equal(t, "", repositories.lastSeenTag)
}

func TestSubscriptionService_Subscribe_ReturnsRepositoryValidationError(t *testing.T) {
	expectedErr := domain.ErrRateLimited
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{}
	repositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{}
	releaseClient := &gitRepositoryProviderStub{existsErr: expectedErr}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.False(t, transactionManager.called)
}

func TestSubscriptionService_Subscribe_ReturnsLatestReleaseError(t *testing.T) {
	expectedErr := errors.New("release lookup failed")
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{}
	repositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{}
	releaseClient := &gitRepositoryProviderStub{releaseErr: expectedErr}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.False(t, transactionManager.called)
}

func TestSubscriptionService_Subscribe_ReturnsSubscriptionStoreError(t *testing.T) {
	expectedErr := errors.New("subscription store failed")
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{user: User{ID: 10, Email: "test@example.com"}}
	repositories := &trackedRepositoryProviderStub{result: releasetracking.TrackedRepository{ID: 20, Owner: "gin-gonic", Name: "gin"}}
	subscriptions := &subscriptionCreatorStub{err: expectedErr}
	releaseClient := &gitRepositoryProviderStub{result: releasetracking.Release{TagName: "v1.11.0"}}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.True(t, transactionManager.called)
	require.True(t, transactionManager.rolledBack)
}

func TestSubscriptionService_Subscribe_ReturnsRepositoryStoreError(t *testing.T) {
	expectedErr := errors.New("repository store failed")
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{user: User{ID: 10, Email: "test@example.com"}}
	repositories := &trackedRepositoryProviderStub{err: expectedErr}
	subscriptions := &subscriptionCreatorStub{}
	releaseClient := &gitRepositoryProviderStub{result: releasetracking.Release{TagName: "v1.11.0"}}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.True(t, transactionManager.called)
	require.True(t, transactionManager.rolledBack)
}

func TestSubscriptionService_Subscribe_ReturnsIncorrectRepositoryFormat(t *testing.T) {
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{}
	repositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{}
	releaseClient := &gitRepositoryProviderStub{}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic")
	require.ErrorIs(t, err, domain.ErrIncorrectRepositoryFormat)
	require.False(t, transactionManager.called)
}

func TestSubscriptionService_Subscribe_ReturnsConfirmationSenderError(t *testing.T) {
	expectedErr := errors.New("confirmation sender failed")
	transactionManager := &transactionManagerStub{}
	users := &userCreatorStub{user: User{ID: 10, Email: "test@example.com"}}
	repositories := &trackedRepositoryProviderStub{result: releasetracking.TrackedRepository{ID: 20, Owner: "gin-gonic", Name: "gin"}}
	subscriptions := &subscriptionCreatorStub{
		result: Subscription{
			ID:                30,
			ConfirmationToken: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	releaseClient := &gitRepositoryProviderStub{result: releasetracking.Release{TagName: "v1.11.0"}}
	mailQueue := &confirmationSenderStub{err: expectedErr}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, mailQueue)

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.True(t, transactionManager.rolledBack)
	require.True(t, mailQueue.called)
}

func TestSubscriptionService_Subscribe_ReturnsTransactionManagerError(t *testing.T) {
	expectedErr := errors.New("commit failed")
	transactionManager := &transactionManagerStub{err: expectedErr}
	users := &userCreatorStub{user: User{ID: 10, Email: "test@example.com"}}
	repositories := &trackedRepositoryProviderStub{result: releasetracking.TrackedRepository{ID: 20, Owner: "gin-gonic", Name: "gin"}}
	subscriptions := &subscriptionCreatorStub{
		result: Subscription{
			ID:                  30,
			UserID:              10,
			TrackedRepositoryID: 20,
			ConfirmationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	releaseClient := &gitRepositoryProviderStub{result: releasetracking.Release{TagName: "v1.11.0"}}

	service := NewService(transactionManager, users, repositories, subscriptions, releaseClient, &confirmationSenderStub{})

	err := service.Subscribe(context.Background(), "test@example.com", "gin-gonic/gin")
	require.ErrorIs(t, err, expectedErr)
	require.True(t, transactionManager.called)
	require.True(t, transactionManager.rolledBack)
}

func TestSubscriptionService_ConfirmSubscription(t *testing.T) {
	subscriptions := &subscriptionCreatorStub{}
	service := NewService(&transactionManagerStub{}, &userCreatorStub{}, &trackedRepositoryProviderStub{}, subscriptions, &gitRepositoryProviderStub{}, &confirmationSenderStub{})

	err := service.ConfirmSubscription(context.Background(), "11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", subscriptions.confirmedToken)
}

func TestSubscriptionService_ConfirmSubscription_ReturnsStoreError(t *testing.T) {
	expectedErr := errors.New("store failed")
	subscriptions := &subscriptionCreatorStub{err: expectedErr}
	service := NewService(&transactionManagerStub{}, &userCreatorStub{}, &trackedRepositoryProviderStub{}, subscriptions, &gitRepositoryProviderStub{}, &confirmationSenderStub{})

	err := service.ConfirmSubscription(context.Background(), "11111111-1111-1111-1111-111111111111")
	require.ErrorIs(t, err, expectedErr)
}

func TestSubscriptionService_CancelSubscription(t *testing.T) {
	subscriptions := &subscriptionCreatorStub{}
	service := NewService(&transactionManagerStub{}, &userCreatorStub{}, &trackedRepositoryProviderStub{}, subscriptions, &gitRepositoryProviderStub{}, &confirmationSenderStub{})

	err := service.CancelSubscription(context.Background(), "22222222-2222-2222-2222-222222222222")
	require.NoError(t, err)
	require.Equal(t, "22222222-2222-2222-2222-222222222222", subscriptions.cancellationToken)
}

func TestSubscriptionService_CancelSubscription_ReturnsStoreError(t *testing.T) {
	expectedErr := errors.New("store failed")
	subscriptions := &subscriptionCreatorStub{err: expectedErr}
	service := NewService(&transactionManagerStub{}, &userCreatorStub{}, &trackedRepositoryProviderStub{}, subscriptions, &gitRepositoryProviderStub{}, &confirmationSenderStub{})

	err := service.CancelSubscription(context.Background(), "22222222-2222-2222-2222-222222222222")
	require.ErrorIs(t, err, expectedErr)
}

func TestSubscriptionService_ListSubscriptions(t *testing.T) {
	subscriptions := &subscriptionCreatorStub{
		listResult: []SubscriptionView{
			{Email: "test@example.com", Repo: "gin-gonic/gin", Confirmed: true, LastSeenTag: "v1.11.0"},
		},
	}
	service := NewService(&transactionManagerStub{}, &userCreatorStub{}, &trackedRepositoryProviderStub{}, subscriptions, &gitRepositoryProviderStub{}, &confirmationSenderStub{})

	result, err := service.ListSubscriptions(context.Background(), "test@example.com")
	require.NoError(t, err)
	require.Equal(t, "test@example.com", subscriptions.listEmail)
	require.Len(t, result, 1)
	require.Equal(t, "gin-gonic/gin", result[0].Repo)
}

func TestSubscriptionService_ListSubscriptions_ReturnsStoreError(t *testing.T) {
	expectedErr := errors.New("store failed")
	subscriptions := &subscriptionCreatorStub{err: expectedErr}
	service := NewService(&transactionManagerStub{}, &userCreatorStub{}, &trackedRepositoryProviderStub{}, subscriptions, &gitRepositoryProviderStub{}, &confirmationSenderStub{})

	result, err := service.ListSubscriptions(context.Background(), "test@example.com")
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, result)
}

func TestSplitRepositoryFullName(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		owner     string
		repoName  string
		expectErr error
	}{
		{name: "valid", input: "gin-gonic/gin", owner: "gin-gonic", repoName: "gin"},
		{name: "missing slash", input: "gin-gonic", expectErr: domain.ErrIncorrectRepositoryFormat},
		{name: "too many parts", input: "a/b/c", expectErr: domain.ErrIncorrectRepositoryFormat},
		{name: "blank owner", input: " /gin", expectErr: domain.ErrIncorrectRepositoryFormat},
		{name: "blank repo", input: "gin/ ", expectErr: domain.ErrIncorrectRepositoryFormat},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repoName, err := splitRepositoryFullName(tc.input)
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.owner, owner)
			require.Equal(t, tc.repoName, repoName)
		})
	}
}
