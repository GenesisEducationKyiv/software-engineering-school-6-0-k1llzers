package storage

import (
	"context"
	"database/sql"

	"github-release-notifier/internal/domain"
)

type TrackedRepositoryStore struct {
	db *sql.DB
}

func NewTrackedRepositoryStore(db *sql.DB) *TrackedRepositoryStore {
	return &TrackedRepositoryStore{db: db}
}

func (s *TrackedRepositoryStore) CreateIfNotExists(ctx context.Context, tx *sql.Tx, owner string, name string, lastSeenTag string) (domain.TrackedRepository, error) {
	query := `
		insert into tracked_repositories (owner, name, last_seen_tag)
		values ($1, $2, $3)
		on conflict (owner, name) 
		do update 
		    set updated_at = now()
		returning id, owner, name, last_seen_tag, created_at, updated_at
	`

	var created domain.TrackedRepository

	err := newQueryExecutor(s.db, tx).QueryRowContext(ctx, query, owner, name, lastSeenTag).Scan(
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

func (s *TrackedRepositoryStore) UpdateLastSeenTag(ctx context.Context, tx *sql.Tx, trackedRepositoryID int64, lastSeenTag string) error {
	query := `
		update tracked_repositories
		set last_seen_tag = $2,
			updated_at = now()
		where id = $1;
	`

	_, err := newQueryExecutor(s.db, tx).ExecContext(ctx, query, trackedRepositoryID, lastSeenTag)
	return err
}
