//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/db/test"

	"github.com/stretchr/testify/require"
)

func TestOpenPostgres(t *testing.T) {
	connStr, rawDB := test.SetupTestPostgres(t)
	require.NoError(t, rawDB.Close())

	db, err := appdb.OpenPostgres(context.Background(), connStr)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	require.NoError(t, db.PingContext(context.Background()))
}

func TestOpenPostgres_ReturnsPingError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	db, err := appdb.OpenPostgres(ctx, "postgres://bad:bad@127.0.0.1:1/missing?sslmode=disable")
	require.Error(t, err)
	require.Nil(t, db)
}
