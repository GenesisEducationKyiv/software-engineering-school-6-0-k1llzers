//go:build unit

package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectMigrationFiles_FiltersAndSorts(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"000_enable_pgcrypto.sql",
		"003_create_subscription.sql",
		"001_create_tracked_repositories.down.sql",
		"002_create_users.sql",
		"001_create_tracked_repositories.sql",
		"README.md",
	}

	for _, file := range files {
		err := os.WriteFile(filepath.Join(dir, file), []byte("select 1;"), 0o644)
		require.NoError(t, err)
	}

	migrations, err := collectMigrationFiles(dir)
	require.NoError(t, err)
	require.Len(t, migrations, 4)
	require.Equal(t, "000_enable_pgcrypto.sql", migrations[0].version)
	require.Equal(t, "001_create_tracked_repositories.sql", migrations[1].version)
	require.Equal(t, "002_create_users.sql", migrations[2].version)
	require.Equal(t, "003_create_subscription.sql", migrations[3].version)
}
