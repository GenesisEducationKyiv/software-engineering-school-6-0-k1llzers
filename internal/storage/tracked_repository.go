package storage

import (
	"context"
	"database/sql"
	"errors"

	"github-release-notifier/internal/domain"
)

type TrackedRepositoryStore struct {
	db *sql.DB
}

func NewTrackedRepositoryStore(db *sql.DB) *TrackedRepositoryStore {
	return &TrackedRepositoryStore{db: db}
}

func (s *TrackedRepositoryStore) Create(ctx context.Context, repository domain.TrackedRepository) (domain.TrackedRepository, error) {
	query := `
		INSERT INTO tracked_repositories (full_name, last_seen_tag)
		VALUES ($1, $2)
		RETURNING id, full_name, last_seen_tag, created_at, updated_at
	`

	var created domain.TrackedRepository

	err := s.db.QueryRowContext(ctx, query, repository.FullName, repository.LastSeenTag).Scan(
		&created.ID,
		&created.FullName,
		&created.LastSeenTag,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return domain.TrackedRepository{}, err
	}

	return created, nil
}

func (s *TrackedRepositoryStore) GetByFullName(ctx context.Context, fullName string) (domain.TrackedRepository, error) {
	query := `
		SELECT id, full_name, last_seen_tag, created_at, updated_at
		FROM tracked_repositories
		WHERE full_name = $1
	`

	var repository domain.TrackedRepository

	err := s.db.QueryRowContext(ctx, query, fullName).Scan(
		&repository.ID,
		&repository.FullName,
		&repository.LastSeenTag,
		&repository.CreatedAt,
		&repository.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.TrackedRepository{}, domain.ErrNotFound
		}

		return domain.TrackedRepository{}, err
	}

	return repository, nil
}

func (s *TrackedRepositoryStore) UpdateLastSeenTag(ctx context.Context, id int64, tag string) error {
	query := `
		UPDATE tracked_repositories
		SET last_seen_tag = $1,
		    updated_at = NOW()
		WHERE id = $2
	`

	result, err := s.db.ExecContext(ctx, query, tag, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}
