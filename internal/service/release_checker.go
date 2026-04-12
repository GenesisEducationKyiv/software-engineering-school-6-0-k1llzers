package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/readmodel"
)

const defaultReleaseCheckInterval = time.Minute

type ReleaseMonitor struct {
	txManager     txManager
	repositories  TrackedRepositoryStore
	subscriptions SubscriptionStore
	repositoryAPI githubRepositoryClient
	mailQueue     mail.Queue
}

func NewReleaseMonitor(
	txManager txManager,
	repositories TrackedRepositoryStore,
	subscriptions SubscriptionStore,
	repositoryAPI githubRepositoryClient,
	mailQueue mail.Queue,
) *ReleaseMonitor {
	return &ReleaseMonitor{
		txManager:     txManager,
		repositories:  repositories,
		subscriptions: subscriptions,
		repositoryAPI: repositoryAPI,
		mailQueue:     mailQueue,
	}
}

func (m *ReleaseMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(defaultReleaseCheckInterval)
	defer ticker.Stop()

	for {
		if err := m.CheckOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("release monitor failed: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *ReleaseMonitor) CheckOnce(ctx context.Context) error {
	subscriptions, err := m.subscriptions.ListConfirmedRepositorySubscriptions(ctx)
	if err != nil {
		return err
	}

	groupedSubscriptions := groupConfirmedSubscriptions(subscriptions)
	var checkErrors []error

	for _, group := range groupedSubscriptions {
		release, err := m.repositoryAPI.GetLatestRelease(ctx, group.owner, group.name)
		if err != nil {
			if errors.Is(err, domain.ErrNoReleases) {
				continue
			}

			checkErrors = append(checkErrors, fmt.Errorf("%s/%s: %w", group.owner, group.name, err))
			continue
		}

		if release.TagName == "" || release.TagName == group.lastSeenTag {
			continue
		}

		releaseURL := buildReleaseURL(group.owner, group.name, release.TagName, release.HTMLURL)
		err = m.txManager.WithinTransaction(ctx, func(tx *sql.Tx) error {
			for _, subscription := range group.subscriptions {
				repositoryFullName := subscription.Owner + "/" + subscription.Name
				if err := m.mailQueue.QueueReleaseNotification(ctx, tx, subscription.Email, repositoryFullName, release.TagName, releaseURL, subscription.CancellationToken); err != nil {
					return err
				}
			}

			return m.repositories.UpdateLastSeenTag(ctx, tx, group.trackedRepositoryID, release.TagName)
		})
		if err != nil {
			checkErrors = append(checkErrors, fmt.Errorf("%s/%s enqueue: %w", group.owner, group.name, err))
		}
	}

	return errors.Join(checkErrors...)
}

type confirmedSubscriptionGroup struct {
	trackedRepositoryID int64
	owner               string
	name                string
	lastSeenTag         string
	subscriptions       []readmodel.ConfirmedRepositorySubscription
}

func groupConfirmedSubscriptions(items []readmodel.ConfirmedRepositorySubscription) []confirmedSubscriptionGroup {
	groupIndexByRepositoryID := make(map[int64]int)
	groups := make([]confirmedSubscriptionGroup, 0)

	for _, item := range items {
		groupIndex, exists := groupIndexByRepositoryID[item.TrackedRepositoryID]
		if !exists {
			groupIndexByRepositoryID[item.TrackedRepositoryID] = len(groups)
			groups = append(groups, confirmedSubscriptionGroup{
				trackedRepositoryID: item.TrackedRepositoryID,
				owner:               item.Owner,
				name:                item.Name,
				lastSeenTag:         item.LastSeenTag,
				subscriptions:       []readmodel.ConfirmedRepositorySubscription{item},
			})
			continue
		}

		groups[groupIndex].subscriptions = append(groups[groupIndex].subscriptions, item)
	}

	return groups
}

func buildReleaseURL(owner string, repo string, tagName string, releaseHTMLURL string) string {
	if releaseHTMLURL != "" {
		return releaseHTMLURL
	}

	return fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tagName)
}
