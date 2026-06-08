package releasetracking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	appmetrics "github-release-notifier/internal/platform/metrics"
	"github-release-notifier/internal/shared"

	"github.com/google/uuid"
)

const (
	defaultReleaseCheckInterval = time.Minute
	defaultRateLimitBackoff     = 15 * time.Minute
)

type txManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type ReleaseMonitor struct {
	txManager     txManager
	repositories  trackedRepositoryTagUpdater
	subscriptions confirmedSubscriptionReader
	repositoryAPI latestReleaseReader
	mailQueue     ReleaseNotificationQueue
	metrics       *appmetrics.Metrics
}

type trackedRepositoryTagUpdater interface {
	UpdateLastSeenTag(ctx context.Context, trackedRepositoryID int64, lastSeenTag string) error
}

type confirmedSubscriptionReader interface {
	ListConfirmedRepositorySubscriptions(ctx context.Context) ([]ConfirmedRepositorySubscription, error)
}

type latestReleaseReader interface {
	GetLatestRelease(ctx context.Context, owner string, repoName string) (Release, error)
}

type ReleaseNotificationQueue interface {
	QueueReleaseNotification(ctx context.Context, recipientEmail string, repositoryFullName string, tagName string, releaseURL string, cancellationToken uuid.UUID) error
}

func NewReleaseMonitor(
	txManager txManager,
	repositories trackedRepositoryTagUpdater,
	subscriptions confirmedSubscriptionReader,
	repositoryAPI latestReleaseReader,
	mailQueue ReleaseNotificationQueue,
	metricSet *appmetrics.Metrics,
) *ReleaseMonitor {
	if metricSet == nil {
		panic("metrics is required")
	}

	return &ReleaseMonitor{
		txManager:     txManager,
		repositories:  repositories,
		subscriptions: subscriptions,
		repositoryAPI: repositoryAPI,
		mailQueue:     mailQueue,
		metrics:       metricSet,
	}
}

func (m *ReleaseMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(defaultReleaseCheckInterval)
	defer ticker.Stop()

	for {
		if err := m.CheckOnce(ctx); err != nil && ctx.Err() == nil {
			if errors.Is(err, shared.ErrRateLimited) {
				slog.WarnContext(ctx, "release monitor hit github rate limit", "backoff", defaultRateLimitBackoff.String(), "error", err)
				if !sleepContext(ctx, defaultRateLimitBackoff) {
					return
				}
			} else {
				slog.ErrorContext(ctx, "release monitor failed", "error", err)
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
	startedAt := time.Now()
	result := "success"
	defer func() {
		m.metrics.ObserveReleaseMonitorCheck(ctx, result, time.Since(startedAt))
	}()

	subscriptions, err := m.subscriptions.ListConfirmedRepositorySubscriptions(ctx)
	if err != nil {
		result = classifyReleaseMonitorResult(err)
		return err
	}

	groupedSubscriptions := groupConfirmedSubscriptions(subscriptions)
	var checkErrors []error

	for _, group := range groupedSubscriptions {
		err := m.processRepositoryRelease(ctx, group)
		if err != nil {
			if errors.Is(err, shared.ErrRateLimited) {
				result = classifyReleaseMonitorResult(err)
				return err
			}
			checkErrors = append(checkErrors, err)
		}
	}

	joinedErr := errors.Join(checkErrors...)
	if joinedErr != nil {
		result = classifyReleaseMonitorResult(joinedErr)
	}

	return joinedErr
}

type confirmedSubscriptionGroup struct {
	trackedRepositoryID int64
	owner               string
	name                string
	lastSeenTag         *string
	subscriptions       []ConfirmedRepositorySubscription
}

func (g confirmedSubscriptionGroup) fullName() string {
	return g.owner + "/" + g.name
}

func (m *ReleaseMonitor) processRepositoryRelease(ctx context.Context, group confirmedSubscriptionGroup) error {
	release, err := m.repositoryAPI.GetLatestRelease(ctx, group.owner, group.name)
	if err != nil {
		return mapLatestReleaseError(group, err)
	}

	if !group.hasInitializedCursor() {
		return m.initializeLastSeenTag(ctx, group, release.TagName)
	}

	if !group.shouldNotify(release.TagName) {
		return nil
	}

	if err := m.queueReleaseNotifications(ctx, group, release); err != nil {
		return fmt.Errorf("%s enqueue: %w", group.fullName(), err)
	}

	return nil
}

func (m *ReleaseMonitor) initializeLastSeenTag(ctx context.Context, group confirmedSubscriptionGroup, tagName string) error {
	if tagName == "" {
		return nil
	}

	if err := m.repositories.UpdateLastSeenTag(ctx, group.trackedRepositoryID, tagName); err != nil {
		return fmt.Errorf("%s initialize cursor: %w", group.fullName(), err)
	}

	return nil
}

func mapLatestReleaseError(group confirmedSubscriptionGroup, err error) error {
	if errors.Is(err, ErrNoReleases) {
		return nil
	}

	return fmt.Errorf("%s: %w", group.fullName(), err)
}

func (m *ReleaseMonitor) queueReleaseNotifications(ctx context.Context, group confirmedSubscriptionGroup, release Release) error {
	releaseURL := buildReleaseURL(group.owner, group.name, release.TagName, release.HTMLURL)

	return m.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		for _, subscription := range group.subscriptions {
			if err := m.mailQueue.QueueReleaseNotification(ctx, subscription.Email, group.fullName(), release.TagName, releaseURL, subscription.CancellationToken); err != nil {
				return err
			}
		}

		return m.repositories.UpdateLastSeenTag(ctx, group.trackedRepositoryID, release.TagName)
	})
}

func (g confirmedSubscriptionGroup) shouldNotify(tagName string) bool {
	return tagName != "" && g.lastSeenTag != nil && tagName != *g.lastSeenTag
}

func (g confirmedSubscriptionGroup) hasInitializedCursor() bool {
	return g.lastSeenTag != nil
}

func groupConfirmedSubscriptions(items []ConfirmedRepositorySubscription) []confirmedSubscriptionGroup {
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
				subscriptions:       []ConfirmedRepositorySubscription{item},
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

func classifyReleaseMonitorResult(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, shared.ErrRateLimited):
		return "rate_limited"
	default:
		return "error"
	}
}
