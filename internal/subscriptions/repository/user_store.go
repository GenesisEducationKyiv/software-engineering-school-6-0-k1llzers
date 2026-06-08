package repository

import (
	"context"
	"database/sql"

	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/subscriptions"
)

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) CreateIfNotExists(ctx context.Context, email string) (subscriptions.User, error) {
	query := `
		insert into users (email)
		values ($1)
		on conflict (email) 
		do update set updated_at = now()
		returning id, email, created_at, updated_at;
	`

	var result subscriptions.User

	err := appdb.NewQueryExecutor(ctx, s.db).QueryRowContext(ctx, query, email).Scan(
		&result.ID,
		&result.Email,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		return subscriptions.User{}, err
	}

	return result, nil
}
