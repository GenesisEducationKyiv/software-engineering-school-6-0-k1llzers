//go:build integration

package storage

import (
	"context"
	"testing"

	dbtx "github-release-notifier/internal/db"

	"github.com/stretchr/testify/require"
)

func TestUserStore_CreateIfNotExists_CreatesUser(t *testing.T) {
	db := setupTestDB(t)
	store := NewUserStore(db)

	ctx := context.Background()
	email := newTestEmail()

	created, err := store.CreateIfNotExists(ctx, email)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, email, created.Email)
	require.False(t, created.CreatedAt.IsZero())
	require.False(t, created.UpdatedAt.IsZero())
}

func TestUserStore_CreateIfNotExists_WhenUserAlreadyExists_ReturnsExistingUser(t *testing.T) {
	db := setupTestDB(t)
	store := NewUserStore(db)

	ctx := context.Background()
	email := newTestEmail()

	first, err := store.CreateIfNotExists(ctx, email)
	require.NoError(t, err)

	second, err := store.CreateIfNotExists(ctx, email)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.Email, second.Email)
	require.Equal(t, first.CreatedAt, second.CreatedAt)
	require.NotEqual(t, first.UpdatedAt, second.UpdatedAt)
}

func TestUserStore_CreateIfNotExists_UsesTransaction(t *testing.T) {
	db := setupTestDB(t)
	store := NewUserStore(db)

	tx := beginTestTx(t, db)
	created, err := store.CreateIfNotExists(dbtx.WithTransactionContext(context.Background(), tx), "tx-user@example.com")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var count int
	err = db.QueryRowContext(context.Background(), `select count(*) from users where id = $1`, created.ID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
