//go:build integration

package test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
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

	return setupTestDBWithMigrations(t, "app")
}

func SetupNotificationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	return setupTestDBWithMigrations(t, "notification")
}

func setupTestDBWithMigrations(t *testing.T, migrationGroup string) *sql.DB {
	t.Helper()

	var db *sql.DB
	switch migrationGroup {
	case "notification":
		_, db = SetupNotificationTestPostgres(t)
	default:
		_, db = SetupTestPostgres(t)
	}
	ctx := context.Background()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "migrations", migrationGroup))
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
