package test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcppostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	sharedPostgresContainerName = "github-release-notifier-test-postgres-v2"
	sharedPostgresUser          = "test"
	sharedPostgresPassword      = "test"
	sharedPostgresDB            = "testdb"
	notificationTestDB          = "notification_testdb"
)

var (
	sharedPostgresOnce      sync.Once
	sharedPostgresContainer *tcppostgres.PostgresContainer
	sharedPostgresErr       error
)

func SetupTestPostgres(t *testing.T) (string, *sql.DB) {
	t.Helper()

	return openSharedTestDatabase(t, sharedPostgresDB)
}

func SetupNotificationTestPostgres(t *testing.T) (string, *sql.DB) {
	t.Helper()

	return openSharedTestDatabase(t, notificationTestDB)
}

func openSharedTestDatabase(t *testing.T, databaseName string) (string, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	container := setupSharedTestPostgresContainer(t)
	adminConnStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	adminDB, err := sql.Open("pgx", adminConnStr)
	require.NoError(t, err)
	require.NoError(t, WaitForDB(ctx, adminDB, 30*time.Second))
	defer func() {
		require.NoError(t, adminDB.Close())
	}()

	connStr, err := connectionStringWithDatabase(adminConnStr, databaseName)
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
			tcppostgres.WithInitScripts(notificationInitScriptPath()),
			tcppostgres.WithUsername(sharedPostgresUser),
			tcppostgres.WithPassword(sharedPostgresPassword),
			testcontainers.WithReuseByName(sharedPostgresContainerName),
		)
	})

	require.NoError(t, sharedPostgresErr)
	return sharedPostgresContainer
}

func notificationInitScriptPath() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve init script path")
	}

	return filepath.Join(filepath.Dir(currentFile), "testdata", "init-notification-db.sql")
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

func connectionStringWithDatabase(connStr string, databaseName string) (string, error) {
	parsed, err := url.Parse(connStr)
	if err != nil {
		return "", err
	}

	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}
