package subscriptions

import (
	"context"
	"errors"
	"strings"

	releasetracking "github-release-notifier/internal/release_tracking"
)

type subscriptionRepository struct {
	owner       string
	name        string
	lastSeenTag string
}

func (s *Service) prepareSubscription(ctx context.Context, repositoryFullName string) (subscriptionRepository, error) {
	owner, repoName, err := splitRepositoryFullName(repositoryFullName)
	if err != nil {
		return subscriptionRepository{}, err
	}

	if err := s.repositoryAPI.RepositoryExists(ctx, owner, repoName); err != nil {
		return subscriptionRepository{}, err
	}

	lastSeenTag, err := s.resolveLastSeenTag(ctx, owner, repoName)
	if err != nil {
		return subscriptionRepository{}, err
	}

	return subscriptionRepository{
		owner:       owner,
		name:        repoName,
		lastSeenTag: lastSeenTag,
	}, nil
}

func (s *Service) resolveLastSeenTag(ctx context.Context, owner string, repoName string) (string, error) {
	release, err := s.repositoryAPI.GetLatestRelease(ctx, owner, repoName)
	if err == nil {
		return release.TagName, nil
	}
	if errors.Is(err, releasetracking.ErrNoReleases) {
		return "", nil
	}

	return "", err
}

func splitRepositoryFullName(repositoryFullName string) (string, string, error) {
	parts := strings.Split(repositoryFullName, "/")

	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", ErrIncorrectRepositoryFormat
	}
	return parts[0], parts[1], nil
}
