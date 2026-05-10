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

const (
	defaultReleaseCheckInterval = time.Minute
	defaultRateLimitBackoff     = 15 * time.Minute
)

type ReleaseMonitor struct {
	txManager     txManager
	repositories  trackedRepositoryTagUpdater
	subscriptions confirmedSubscriptionReader
	repositoryAPI latestReleaseReader
	mailQueue     mail.ReleaseNotificationQueue
}

type trackedRepositoryTagUpdater interface {
	UpdateLastSeenTag(ctx context.Context, tx *sql.Tx, trackedRepositoryID int64, lastSeenTag string) error
}

type confirmedSubscriptionReader interface {
	ListConfirmedRepositorySubscriptions(ctx context.Context) ([]readmodel.ConfirmedRepositorySubscription, error)
}

type latestReleaseReader interface {
	GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error)
}

func NewReleaseMonitor(
	txManager txManager,
	repositories trackedRepositoryTagUpdater,
	subscriptions confirmedSubscriptionReader,
	repositoryAPI latestReleaseReader,
	mailQueue mail.ReleaseNotificationQueue,
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
			if errors.Is(err, domain.ErrRateLimited) {
				log.Printf("release monitor hit github rate limit, pausing for %s: %v", defaultRateLimitBackoff, err)
				if !sleepContext(ctx, defaultRateLimitBackoff) {
					return
				}
			} else {
				log.Printf("release monitor failed: %v", err)
			}
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
		err := m.processRepositoryRelease(ctx, group)
		if err != nil {
			if errors.Is(err, domain.ErrRateLimited) {
				return err
			}
			checkErrors = append(checkErrors, err)
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

func (g confirmedSubscriptionGroup) fullName() string {
	return g.owner + "/" + g.name
}

func (m *ReleaseMonitor) processRepositoryRelease(ctx context.Context, group confirmedSubscriptionGroup) error {
	release, err := m.repositoryAPI.GetLatestRelease(ctx, group.owner, group.name)
	if err != nil {
		return mapLatestReleaseError(group, err)
	}

	if !group.shouldNotify(release.TagName) {
		return nil
	}

	if err := m.queueReleaseNotifications(ctx, group, release); err != nil {
		return fmt.Errorf("%s enqueue: %w", group.fullName(), err)
	}

	return nil
}

func mapLatestReleaseError(group confirmedSubscriptionGroup, err error) error {
	if errors.Is(err, domain.ErrNoReleases) {
		return nil
	}

	return fmt.Errorf("%s: %w", group.fullName(), err)
}

func (m *ReleaseMonitor) queueReleaseNotifications(ctx context.Context, group confirmedSubscriptionGroup, release domain.Release) error {
	releaseURL := buildReleaseURL(group.owner, group.name, release.TagName, release.HTMLURL)

	return m.txManager.WithinTransaction(ctx, func(tx *sql.Tx) error {
		for _, subscription := range group.subscriptions {
			if err := m.mailQueue.QueueReleaseNotification(ctx, tx, subscription.Email, group.fullName(), release.TagName, releaseURL, subscription.CancellationToken); err != nil {
				return err
			}
		}

		return m.repositories.UpdateLastSeenTag(ctx, tx, group.trackedRepositoryID, release.TagName)
	})
}

func (g confirmedSubscriptionGroup) shouldNotify(tagName string) bool {
	return tagName != "" && tagName != g.lastSeenTag
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

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
