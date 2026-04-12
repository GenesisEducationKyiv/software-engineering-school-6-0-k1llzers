package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github-release-notifier/internal/domain"
)

const defaultReleaseCheckInterval = time.Minute

type ReleaseCheckerService struct {
	transactionManager    transactionManager
	trackedRepositories   trackedRepositoryProvider
	subscriptions         subscriptionCreator
	gitRepositoryProvider gitRepositoryProvider
	notifications         notificationQueueFactory
}

func NewReleaseCheckerService(
	transactionManager transactionManager,
	trackedRepositories trackedRepositoryProvider,
	subscriptions subscriptionCreator,
	gitRepositoryProvider gitRepositoryProvider,
	notifications notificationQueueFactory,
) *ReleaseCheckerService {
	return &ReleaseCheckerService{
		transactionManager:    transactionManager,
		trackedRepositories:   trackedRepositories,
		subscriptions:         subscriptions,
		gitRepositoryProvider: gitRepositoryProvider,
		notifications:         notifications,
	}
}

func (s *ReleaseCheckerService) Run(ctx context.Context) {
	ticker := time.NewTicker(defaultReleaseCheckInterval)
	defer ticker.Stop()

	for {
		if err := s.CheckOnce(ctx); err != nil && ctx.Err() == nil {
			fmt.Printf("release check failed: %v\n", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *ReleaseCheckerService) CheckOnce(ctx context.Context) error {
	subscriptions, err := s.subscriptions.ListConfirmedRepositorySubscriptions(ctx)
	if err != nil {
		return err
	}

	grouped := groupConfirmedSubscriptions(subscriptions)
	var errs []error

	for _, group := range grouped {
		release, err := s.gitRepositoryProvider.GetLatestRelease(ctx, group.owner, group.name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s/%s: %w", group.owner, group.name, err))
			continue
		}

		if release.TagName == "" || release.TagName == group.lastSeenTag {
			continue
		}

		releaseURL := buildReleaseURL(group.owner, group.name, release.TagName, release.HTMLURL)
		err = s.transactionManager.WithinTransaction(ctx, func(tx *sql.Tx) error {
			trackedRepositories := s.trackedRepositories.WithTx(tx)
			notifications := s.notifications.WithTx(tx)

			for _, subscription := range group.subscriptions {
				repositoryFullName := subscription.Owner + "/" + subscription.Name
				if err := notifications.QueueReleaseNotification(ctx, subscription.Email, repositoryFullName, release.TagName, releaseURL, subscription.CancellationToken); err != nil {
					return err
				}
			}

			return trackedRepositories.UpdateLastSeenTag(ctx, group.trackedRepositoryID, release.TagName)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s/%s enqueue: %w", group.owner, group.name, err))
		}
	}

	return errors.Join(errs...)
}

type confirmedSubscriptionGroup struct {
	trackedRepositoryID int64
	owner               string
	name                string
	lastSeenTag         string
	subscriptions       []domain.ConfirmedRepositorySubscription
}

func groupConfirmedSubscriptions(items []domain.ConfirmedRepositorySubscription) []confirmedSubscriptionGroup {
	indexByRepoID := make(map[int64]int)
	groups := make([]confirmedSubscriptionGroup, 0)

	for _, item := range items {
		index, ok := indexByRepoID[item.TrackedRepositoryID]
		if !ok {
			indexByRepoID[item.TrackedRepositoryID] = len(groups)
			groups = append(groups, confirmedSubscriptionGroup{
				trackedRepositoryID: item.TrackedRepositoryID,
				owner:               item.Owner,
				name:                item.Name,
				lastSeenTag:         item.LastSeenTag,
				subscriptions:       []domain.ConfirmedRepositorySubscription{item},
			})
			continue
		}

		groups[index].subscriptions = append(groups[index].subscriptions, item)
	}

	return groups
}

func buildReleaseURL(owner string, repo string, tagName string, releaseHTMLURL string) string {
	if releaseHTMLURL != "" {
		return releaseHTMLURL
	}

	return fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tagName)
}
