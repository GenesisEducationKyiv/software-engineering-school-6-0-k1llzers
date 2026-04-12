package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	storagedb "github-release-notifier/internal/db"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcppostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := tcppostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcppostgres.WithDatabase("testdb"),
		tcppostgres.WithUsername("test"),
		tcppostgres.WithPassword("test"),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	testcontainers.CleanupContainer(t, pgContainer)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	if err := waitForDB(ctx, db, 30*time.Second); err != nil {
		t.Fatalf("db is not ready: %v", err)
	}

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

func waitForDB(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}

		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("timed out waiting for db")
}
