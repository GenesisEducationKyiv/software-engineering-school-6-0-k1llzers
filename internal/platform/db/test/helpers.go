//go:build integration

package test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	appdb "github-release-notifier/internal/platform/db"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type Repository struct {
	Owner string
	Name  string
}

func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	_, db := SetupTestPostgres(t)
	ctx := context.Background()

	migrationsDir := filepath.Join("..", "..", "..", "..", "migrations")
	require.NoError(t, appdb.RunMigrations(ctx, db, migrationsDir))

	return db
}

func BeginTestTx(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	return tx
}

func NewTestEmail() string {
	return fmt.Sprintf("test-%s@example.com", uuid.NewString())
}

func NewTestRepositoryWithPrefix(prefix string) Repository {
	id := uuid.NewString()[:8]
	return Repository{
		Owner: fmt.Sprintf("%s-owner-%s", prefix, id),
		Name:  fmt.Sprintf("%s-repo-%s", prefix, id),
	}
}

func NewTestRepository() Repository {
	return NewTestRepositoryWithPrefix("test")
}
