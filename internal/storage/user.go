package storage

import (
	"context"
	"database/sql"

	"github-release-notifier/internal/domain"
)

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) CreateIfNotExists(ctx context.Context, tx *sql.Tx, email string) (domain.User, error) {
	query := `
		insert into users (email)
		values ($1)
		on conflict (email) 
		do update set updated_at = now()
		returning id, email, created_at, updated_at;
	`

	var result domain.User

	err := newQueryExecutor(s.db, tx).QueryRowContext(ctx, query, email).Scan(
		&result.ID,
		&result.Email,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		return domain.User{}, err
	}

	return result, nil
}
