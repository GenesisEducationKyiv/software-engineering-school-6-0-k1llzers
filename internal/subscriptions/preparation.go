package subscriptions

import (
	"context"
	"strings"
)

type subscriptionRepository struct {
	owner string
	name  string
}

func (s *Service) prepareSubscription(ctx context.Context, repositoryFullName string) (subscriptionRepository, error) {
	owner, repoName, err := splitRepositoryFullName(repositoryFullName)
	if err != nil {
		return subscriptionRepository{}, err
	}

	if err := s.repositoryAPI.RepositoryExists(ctx, owner, repoName); err != nil {
		return subscriptionRepository{}, err
	}

	return subscriptionRepository{
		owner: owner,
		name:  repoName,
	}, nil
}

func splitRepositoryFullName(repositoryFullName string) (string, string, error) {
	parts := strings.Split(repositoryFullName, "/")

	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", ErrIncorrectRepositoryFormat
	}
	return parts[0], parts[1], nil
}
