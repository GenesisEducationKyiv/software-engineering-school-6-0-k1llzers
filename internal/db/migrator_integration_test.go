//go:build integration

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunMigrations_AppliesAndSkipsAlreadyApplied(t *testing.T) {
	_, db := setupTestPostgres(t)
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "001_create_alpha.sql"), []byte(`create table alpha (id integer primary key);`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "002_insert_alpha.sql"), []byte(`insert into alpha (id) values (1);`), 0o644))

	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, db, dir))
	require.NoError(t, RunMigrations(ctx, db, dir))

	var migrationCount int
	require.NoError(t, db.QueryRowContext(ctx, `select count(*) from schema_migrations`).Scan(&migrationCount))
	require.Equal(t, 2, migrationCount)

	var rowCount int
	require.NoError(t, db.QueryRowContext(ctx, `select count(*) from alpha`).Scan(&rowCount))
	require.Equal(t, 1, rowCount)
}

func TestRunMigrations_RollsBackFailedMigration(t *testing.T) {
	_, db := setupTestPostgres(t)
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "001_create_alpha.sql"), []byte(`create table alpha (id integer primary key);`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "002_invalid.sql"), []byte(`insert into missing_table (id) values (1);`), 0o644))

	err := RunMigrations(context.Background(), db, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "apply migration 002_invalid.sql")

	var migrationCount int
	require.NoError(t, db.QueryRowContext(context.Background(), `select count(*) from schema_migrations`).Scan(&migrationCount))
	require.Equal(t, 1, migrationCount)

	var failedMigrationCount int
	require.NoError(t, db.QueryRowContext(context.Background(), `select count(*) from schema_migrations where version = '002_invalid.sql'`).Scan(&failedMigrationCount))
	require.Zero(t, failedMigrationCount)
}
