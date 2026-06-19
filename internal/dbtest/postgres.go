package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcppostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	sharedPostgresContainerName = "github-release-notifier-test-postgres"
	sharedPostgresUser          = "test"
	sharedPostgresPassword      = "test"
	sharedPostgresDB            = "testdb"
)

var (
	sharedPostgresOnce      sync.Once
	sharedPostgresContainer *tcppostgres.PostgresContainer
	sharedPostgresErr       error
)

func SetupTestPostgres(t *testing.T) (string, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	container := setupSharedTestPostgresContainer(t)
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	require.NoError(t, WaitForDB(ctx, db, 30*time.Second))

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return connStr, db
}

func SetupFreshTestPostgres(t *testing.T) (string, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	pgContainer, err := tcppostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcppostgres.WithDatabase("testdb"),
		tcppostgres.WithUsername("test"),
		tcppostgres.WithPassword("test"),
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, pgContainer)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	require.NoError(t, WaitForDB(ctx, db, 30*time.Second))
	return connStr, db
}

func setupSharedTestPostgresContainer(t *testing.T) *tcppostgres.PostgresContainer {
	t.Helper()

	sharedPostgresOnce.Do(func() {
		sharedPostgresContainer, sharedPostgresErr = tcppostgres.Run(
			context.Background(),
			"postgres:16-alpine",
			tcppostgres.WithDatabase(sharedPostgresDB),
			tcppostgres.WithUsername(sharedPostgresUser),
			tcppostgres.WithPassword(sharedPostgresPassword),
			testcontainers.WithReuseByName(sharedPostgresContainerName),
		)
	})

	require.NoError(t, sharedPostgresErr)
	return sharedPostgresContainer
}

func WaitForDB(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("timed out waiting for db")
}
