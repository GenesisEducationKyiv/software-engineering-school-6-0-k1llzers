package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrackedRepositoryStore_CreateIfNotExists_CreatesRepository(t *testing.T) {
	db := setupTestDB(t)
	store := NewTrackedRepositoryStore(db)

	ctx := context.Background()
	created, err := store.CreateIfNotExists(ctx, nil, "gin-gonic", "gin", "v1.0.0")
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "gin-gonic", created.Owner)
	require.Equal(t, "gin", created.Name)
	require.NotNil(t, created.LastSeenTag)
	require.Equal(t, "v1.0.0", *created.LastSeenTag)
}

func TestTrackedRepositoryStore_CreateIfNotExists_ReturnsExistingRepositoryWithoutOverwritingTag(t *testing.T) {
	db := setupTestDB(t)
	store := NewTrackedRepositoryStore(db)

	ctx := context.Background()
	first, err := store.CreateIfNotExists(ctx, nil, "gin-gonic", "gin", "v1.0.0")
	require.NoError(t, err)

	second, err := store.CreateIfNotExists(ctx, nil, "gin-gonic", "gin", "v2.0.0")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.NotNil(t, second.LastSeenTag)
	require.Equal(t, "v1.0.0", *second.LastSeenTag)
	require.NotEqual(t, first.UpdatedAt, second.UpdatedAt)
}

func TestTrackedRepositoryStore_UpdateLastSeenTag_UpdatesRepository(t *testing.T) {
	db := setupTestDB(t)
	store := NewTrackedRepositoryStore(db)

	ctx := context.Background()
	repository, err := store.CreateIfNotExists(ctx, nil, "gin-gonic", "gin", "")
	require.NoError(t, err)

	err = store.UpdateLastSeenTag(ctx, nil, repository.ID, "v1.11.0")
	require.NoError(t, err)

	var lastSeenTag string
	err = db.QueryRowContext(ctx, `select coalesce(last_seen_tag, '') from tracked_repositories where id = $1`, repository.ID).Scan(&lastSeenTag)
	require.NoError(t, err)
	require.Equal(t, "v1.11.0", lastSeenTag)
}

func TestTrackedRepositoryStore_MethodsUseTransaction(t *testing.T) {
	db := setupTestDB(t)
	store := NewTrackedRepositoryStore(db)

	ctx := context.Background()
	tx := beginTestTx(t, db)

	repository, err := store.CreateIfNotExists(ctx, tx, "labstack", "echo", "")
	require.NoError(t, err)

	err = store.UpdateLastSeenTag(ctx, tx, repository.ID, "v4.13.4")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var lastSeenTag string
	err = db.QueryRowContext(ctx, `select coalesce(last_seen_tag, '') from tracked_repositories where id = $1`, repository.ID).Scan(&lastSeenTag)
	require.NoError(t, err)
	require.Equal(t, "v4.13.4", lastSeenTag)
}
