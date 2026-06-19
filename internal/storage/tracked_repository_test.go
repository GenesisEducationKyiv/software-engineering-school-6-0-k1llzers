//go:build integration

package storage

import (
	"context"
	"testing"

	dbtx "github-release-notifier/internal/db"

	"github.com/stretchr/testify/require"
)

func TestTrackedRepositoryStore_CreateIfNotExists_CreatesRepository(t *testing.T) {
	db := setupTestDB(t)
	store := NewTrackedRepositoryStore(db)

	ctx := context.Background()
	repository := newTestRepository()
	created, err := store.CreateIfNotExists(ctx, repository.Owner, repository.Name, "v1.0.0")
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, repository.Owner, created.Owner)
	require.Equal(t, repository.Name, created.Name)
	require.NotNil(t, created.LastSeenTag)
	require.Equal(t, "v1.0.0", *created.LastSeenTag)
}

func TestTrackedRepositoryStore_CreateIfNotExists_ReturnsExistingRepositoryWithoutOverwritingTag(t *testing.T) {
	db := setupTestDB(t)
	store := NewTrackedRepositoryStore(db)

	ctx := context.Background()
	repository := newTestRepository()
	first, err := store.CreateIfNotExists(ctx, repository.Owner, repository.Name, "v1.0.0")
	require.NoError(t, err)

	second, err := store.CreateIfNotExists(ctx, repository.Owner, repository.Name, "v2.0.0")
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
	testRepository := newTestRepository()
	repository, err := store.CreateIfNotExists(ctx, testRepository.Owner, testRepository.Name, "")
	require.NoError(t, err)

	err = store.UpdateLastSeenTag(ctx, repository.ID, "v1.11.0")
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

	txCtx := dbtx.WithTransactionContext(ctx, tx)
	testRepository := newTestRepository()
	repository, err := store.CreateIfNotExists(txCtx, testRepository.Owner, testRepository.Name, "")
	require.NoError(t, err)

	err = store.UpdateLastSeenTag(txCtx, repository.ID, "v4.13.4")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var lastSeenTag string
	err = db.QueryRowContext(ctx, `select coalesce(last_seen_tag, '') from tracked_repositories where id = $1`, repository.ID).Scan(&lastSeenTag)
	require.NoError(t, err)
	require.Equal(t, "v4.13.4", lastSeenTag)
}
