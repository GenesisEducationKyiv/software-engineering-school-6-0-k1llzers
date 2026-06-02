//go:build integration

package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	storagedb "github-release-notifier/internal/db"
	"github-release-notifier/internal/dbtest"

	"github.com/stretchr/testify/require"
)

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
