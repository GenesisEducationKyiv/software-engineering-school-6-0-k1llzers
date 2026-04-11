package storage

import (
	"context"
	"database/sql"
	"github-release-notifier/internal/domain"
)

type TrackedRepositoryStore struct {
	executor executor
}

func NewTrackedRepositoryStore(db *sql.DB) *TrackedRepositoryStore {
	return &TrackedRepositoryStore{executor: db}
}

func (s *TrackedRepositoryStore) WithTx(tx *sql.Tx) *TrackedRepositoryStore {
	return &TrackedRepositoryStore{executor: tx}
}

func (s *TrackedRepositoryStore) CreatIfNotExists(ctx context.Context, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error) {
	query := `
		insert into tracked_repositories (owner, name, last_seen_tag)
		values ($1, $2, $3)
		on conflict (owner, name) 
		do update 
		    set last_seen_tag = $3
		returning id, owner, name, last_seen_tag, created_at, updated_at
	`

	var created domain.TrackedRepository

	err := s.executor.QueryRowContext(ctx, query, owner, name, lastSeenTag).Scan(
		&created.ID,
		&created.Owner,
		&created.Name,
		&created.LastSeenTag,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return domain.TrackedRepository{}, err
	}

	return created, nil
}
