package storage

import (
	"context"
	"database/sql"

	"github-release-notifier/internal/domain"
)

type UserStore struct {
	executor executor
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{executor: db}
}

func (s *UserStore) WithTx(tx *sql.Tx) *UserStore {
	return &UserStore{executor: tx}
}

func (s *UserStore) CreateIfNotExists(ctx context.Context, email string) (domain.User, error) {
	query := `
		insert into users (email)
		values ($1)
		on conflict (email) 
		do update set updated_at = now()
		returning id, email, created_at, updated_at;
	`

	var result domain.User

	err := s.executor.QueryRowContext(ctx, query, email).Scan(
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
