package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/readmodel"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReleaseMonitor_CheckOnce_QueuesEmailsAndUpdatesTag(t *testing.T) {
	transactionManager := &transactionManagerStub{}
	trackedRepositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "second@example.com",
				CancellationToken:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			},
		},
	}
	gitRepositories := &gitRepositoryProviderStub{
		result: domain.Release{TagName: "v1.11.0"},
	}
	notifications := &notificationQueueFactoryStub{}

	service := NewReleaseMonitor(
		transactionManager,
		trackedRepositories,
		subscriptions,
		gitRepositories,
		notifications,
	)

	err := service.CheckOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, "gin-gonic", gitRepositories.owner)
	require.Equal(t, "gin", gitRepositories.repoName)
	require.Len(t, notifications.releaseCalls, 2)
	require.Equal(t, "gin-gonic/gin", notifications.releaseCalls[0].repositoryFullName)
	require.Equal(t, "v1.11.0", notifications.releaseCalls[0].tagName)
	require.Equal(t, int64(10), trackedRepositories.updatedID)
	require.Equal(t, "v1.11.0", trackedRepositories.updatedTag)
	require.True(t, transactionManager.committed)
}

func TestReleaseMonitor_CheckOnce_SkipsSameTag(t *testing.T) {
	transactionManager := &transactionManagerStub{}
	trackedRepositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.11.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
		},
	}
	gitRepositories := &gitRepositoryProviderStub{
		result: domain.Release{TagName: "v1.11.0"},
	}
	notifications := &notificationQueueFactoryStub{}

	service := NewReleaseMonitor(
		transactionManager,
		trackedRepositories,
		subscriptions,
		gitRepositories,
		notifications,
	)

	err := service.CheckOnce(context.Background())
	require.NoError(t, err)
	require.Empty(t, notifications.releaseCalls)
	require.Zero(t, trackedRepositories.updatedID)
	require.False(t, transactionManager.called)
}

func TestReleaseMonitor_CheckOnce_ReturnsTransactionError(t *testing.T) {
	expectedErr := errors.New("queue failed")
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
		},
	}
	service := NewReleaseMonitor(
		&transactionManagerStub{},
		&trackedRepositoryProviderStub{},
		subscriptions,
		&gitRepositoryProviderStub{result: domain.Release{TagName: "v1.11.0"}},
		&notificationQueueFactoryStub{queueErr: expectedErr},
	)

	err := service.CheckOnce(context.Background())
	require.ErrorIs(t, err, expectedErr)
}

func TestReleaseMonitor_CheckOnce_ReturnsListError(t *testing.T) {
	expectedErr := errors.New("list failed")
	service := NewReleaseMonitor(
		&transactionManagerStub{},
		&trackedRepositoryProviderStub{},
		&subscriptionCreatorStub{err: expectedErr},
		&gitRepositoryProviderStub{},
		&notificationQueueFactoryStub{},
	)

	err := service.CheckOnce(context.Background())
	require.ErrorIs(t, err, expectedErr)
}

func TestReleaseMonitor_CheckOnce_SkipsRepositoryWithoutReleases(t *testing.T) {
	transactionManager := &transactionManagerStub{}
	trackedRepositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
		},
	}
	gitRepositories := &gitRepositoryProviderStub{releaseErr: domain.ErrNoReleases}
	notifications := &notificationQueueFactoryStub{}

	service := NewReleaseMonitor(
		transactionManager,
		trackedRepositories,
		subscriptions,
		gitRepositories,
		notifications,
	)

	err := service.CheckOnce(context.Background())
	require.NoError(t, err)
	require.Empty(t, notifications.releaseCalls)
	require.False(t, transactionManager.called)
}

func TestReleaseMonitor_CheckOnce_ReturnsJoinedRepositoryErrors(t *testing.T) {
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
			{
				TrackedRepositoryID: 20,
				Owner:               "labstack",
				Name:                "echo",
				LastSeenTag:         "v4.13.3",
				Email:               "second@example.com",
				CancellationToken:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			},
		},
	}
	expectedErr := errors.New("github failed")
	service := NewReleaseMonitor(
		&transactionManagerStub{},
		&trackedRepositoryProviderStub{},
		subscriptions,
		&gitRepositoryProviderStub{releaseErr: expectedErr},
		&notificationQueueFactoryStub{},
	)

	err := service.CheckOnce(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, expectedErr)
	require.Contains(t, err.Error(), "gin-gonic/gin")
	require.Contains(t, err.Error(), "labstack/echo")
}

func TestReleaseMonitor_CheckOnce_StopsOnRateLimit(t *testing.T) {
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
			{
				TrackedRepositoryID: 20,
				Owner:               "labstack",
				Name:                "echo",
				LastSeenTag:         "v4.13.3",
				Email:               "second@example.com",
				CancellationToken:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			},
		},
	}
	gitRepositories := &gitRepositoryProviderStub{releaseErr: domain.ErrRateLimited}
	service := NewReleaseMonitor(
		&transactionManagerStub{},
		&trackedRepositoryProviderStub{},
		subscriptions,
		gitRepositories,
		&notificationQueueFactoryStub{},
	)

	err := service.CheckOnce(context.Background())
	require.ErrorIs(t, err, domain.ErrRateLimited)
	require.Equal(t, 1, gitRepositories.calls)
	require.Contains(t, err.Error(), "gin-gonic/gin")
}

func TestReleaseMonitor_CheckOnce_UsesReleaseHTMLURLWhenPresent(t *testing.T) {
	transactionManager := &transactionManagerStub{}
	trackedRepositories := &trackedRepositoryProviderStub{}
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
		},
	}
	gitRepositories := &gitRepositoryProviderStub{
		result: domain.Release{
			TagName: "v1.11.0",
			HTMLURL: "https://example.com/custom-release-url",
		},
	}
	notifications := &notificationQueueFactoryStub{}

	service := NewReleaseMonitor(
		transactionManager,
		trackedRepositories,
		subscriptions,
		gitRepositories,
		notifications,
	)

	err := service.CheckOnce(context.Background())
	require.NoError(t, err)
	require.Len(t, notifications.releaseCalls, 1)
	require.Equal(t, "https://example.com/custom-release-url", notifications.releaseCalls[0].releaseURL)
}

func TestReleaseMonitor_CheckOnce_ReturnsUpdateLastSeenTagError(t *testing.T) {
	expectedErr := errors.New("update failed")
	transactionManager := &transactionManagerStub{}
	trackedRepositories := &trackedRepositoryProviderStub{err: expectedErr}
	subscriptions := &subscriptionCreatorStub{
		confirmedList: []readmodel.ConfirmedRepositorySubscription{
			{
				TrackedRepositoryID: 10,
				Owner:               "gin-gonic",
				Name:                "gin",
				LastSeenTag:         "v1.10.0",
				Email:               "first@example.com",
				CancellationToken:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
		},
	}

	service := NewReleaseMonitor(
		transactionManager,
		trackedRepositories,
		subscriptions,
		&gitRepositoryProviderStub{result: domain.Release{TagName: "v1.11.0"}},
		&notificationQueueFactoryStub{},
	)

	err := service.CheckOnce(context.Background())
	require.ErrorIs(t, err, expectedErr)
}

func TestGroupConfirmedSubscriptions(t *testing.T) {
	items := []readmodel.ConfirmedRepositorySubscription{
		{
			TrackedRepositoryID: 10,
			Owner:               "gin-gonic",
			Name:                "gin",
			LastSeenTag:         "v1.11.0",
			Email:               "first@example.com",
		},
		{
			TrackedRepositoryID: 10,
			Owner:               "gin-gonic",
			Name:                "gin",
			LastSeenTag:         "v1.11.0",
			Email:               "second@example.com",
		},
		{
			TrackedRepositoryID: 20,
			Owner:               "labstack",
			Name:                "echo",
			LastSeenTag:         "v4.13.4",
			Email:               "third@example.com",
		},
	}

	groups := groupConfirmedSubscriptions(items)
	require.Len(t, groups, 2)
	require.Equal(t, int64(10), groups[0].trackedRepositoryID)
	require.Len(t, groups[0].subscriptions, 2)
	require.Equal(t, int64(20), groups[1].trackedRepositoryID)
	require.Len(t, groups[1].subscriptions, 1)
}

func TestBuildReleaseURL(t *testing.T) {
	require.Equal(
		t,
		"https://example.com/release",
		buildReleaseURL("gin-gonic", "gin", "v1.11.0", "https://example.com/release"),
	)

	require.Equal(
		t,
		"https://github.com/gin-gonic/gin/releases/tag/v1.11.0",
		buildReleaseURL("gin-gonic", "gin", "v1.11.0", ""),
	)
}

func TestSleepContext_ReturnsFalseWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.False(t, sleepContext(ctx, time.Second))
}

type releaseCall struct {
	recipientEmail     string
	repositoryFullName string
	tagName            string
	releaseURL         string
	cancellationToken  uuid.UUID
}

type notificationQueueFactoryStub struct {
	queueErr      error
	releaseCalls  []releaseCall
	confirmations int
}

func (s *notificationQueueFactoryStub) QueueSubscriptionConfirmation(_ context.Context, _ string, _ string, _ uuid.UUID, _ uuid.UUID) error {
	s.confirmations++
	return s.queueErr
}

func (s *notificationQueueFactoryStub) QueueReleaseNotification(_ context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error {
	if s.queueErr != nil {
		return s.queueErr
	}

	s.releaseCalls = append(s.releaseCalls, releaseCall{
		recipientEmail:     recipientEmail,
		repositoryFullName: repositoryFullName,
		tagName:            tagName,
		releaseURL:         releaseURL,
		cancellationToken:  cancellationToken,
	})
	return nil
}
