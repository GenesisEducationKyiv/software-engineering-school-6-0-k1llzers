//go:build integration

package db

import (
	"database/sql"
	"testing"

	"github-release-notifier/internal/dbtest"
)

func setupTestPostgres(t *testing.T) (string, *sql.DB) {
	t.Helper()
	return dbtest.SetupTestPostgres(t)
}
