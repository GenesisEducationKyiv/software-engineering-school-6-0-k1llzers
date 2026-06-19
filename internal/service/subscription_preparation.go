package service

import (
	"context"
	"errors"
	"strings"

	"github-release-notifier/internal/domain"
)

type githubRepositoryClient interface {
	RepositoryExists(ctx context.Context, owner string, repoName string) error
	GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error)
}

type subscriptionRepository struct {
	owner       string
	name        string
	lastSeenTag string
}

func (s *SubscriptionService) prepareSubscription(ctx context.Context, repositoryFullName string) (subscriptionRepository, error) {
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

func (s *SubscriptionService) resolveLastSeenTag(ctx context.Context, owner string, repoName string) (string, error) {
	release, err := s.repositoryAPI.GetLatestRelease(ctx, owner, repoName)
	if err == nil {
		return release.TagName, nil
	}
	if errors.Is(err, domain.ErrNoReleases) {
		return "", nil
	}

	return "", err
}

func splitRepositoryFullName(repositoryFullName string) (string, string, error) {
	parts := strings.Split(repositoryFullName, "/")

	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", domain.ErrIncorrectRepositoryFormat
	}
	return parts[0], parts[1], nil
}
