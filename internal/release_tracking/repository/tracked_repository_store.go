package repository

import (
	"context"
	"database/sql"

	appdb "github-release-notifier/internal/platform/db"
	releasetracking "github-release-notifier/internal/release_tracking"
)

type TrackedRepositoryStore struct {
	db *sql.DB
}

func NewTrackedRepositoryStore(db *sql.DB) *TrackedRepositoryStore {
	return &TrackedRepositoryStore{db: db}
}

func (s *TrackedRepositoryStore) CreateIfNotExists(ctx context.Context, owner string, name string) (releasetracking.TrackedRepository, error) {
	query := `
		insert into tracked_repositories (owner, name)
		values ($1, $2)
		on conflict (owner, name) 
		do update 
		    set updated_at = now()
		returning id, owner, name, last_seen_tag, created_at, updated_at
	`

	var created releasetracking.TrackedRepository
	var lastSeenTag sql.NullString

	err := appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, owner, name).Scan(
		&created.ID,
		&created.Owner,
		&created.Name,
		&lastSeenTag,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return releasetracking.TrackedRepository{}, err
	}

	if lastSeenTag.Valid {
		created.LastSeenTag = new(lastSeenTag.String)
	}

	return created, nil
}

func (s *TrackedRepositoryStore) UpdateLastSeenTag(ctx context.Context, trackedRepositoryID int64, lastSeenTag string) error {
	query := `
		update tracked_repositories
		set last_seen_tag = $2,
			updated_at = now()
		where id = $1;
	`

	_, err := appdb.NewQueryExecutor(ctx, s.db).ExecContext(ctx, query, trackedRepositoryID, lastSeenTag)
	return err
}
