package storage

import (
	"context"
	"database/sql"
	"github-release-notifier/internal/domain"
)

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{db} }

func (s *UserStore) CreateIfNotExists(ctx context.Context, email string) (domain.User, error) {
	query := `
		insert into users (email)
		values ($1)
		on conflict (email) do update set email = $1
		returning id, email, created_at, updated_at;
	`

	var result domain.User

	err := s.db.QueryRowContext(ctx, query, email).Scan(
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
