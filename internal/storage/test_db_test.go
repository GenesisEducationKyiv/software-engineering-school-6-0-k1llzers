//go:build integration

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	storagedb "github-release-notifier/internal/db"
	"github-release-notifier/internal/dbtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testRepository struct {
	Owner string
	Name  string
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	_, db := dbtest.SetupTestPostgres(t)
	ctx := context.Background()

	migrationsDir := filepath.Join("..", "..", "migrations")
	require.NoError(t, storagedb.RunMigrations(ctx, db, migrationsDir))

	return db
}

func beginTestTx(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	return tx
}

func newTestEmail() string {
	return fmt.Sprintf("test-%s@example.com", uuid.NewString())
}

func newTestRepositoryWithPrefix(prefix string) testRepository {
	id := uuid.NewString()[:8]
	return testRepository{
		Owner: fmt.Sprintf("%s-owner-%s", prefix, id),
		Name:  fmt.Sprintf("%s-repo-%s", prefix, id),
	}
}

func newTestRepository() testRepository {
	return newTestRepositoryWithPrefix("test")
}
