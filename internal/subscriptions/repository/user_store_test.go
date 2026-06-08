//go:build integration

package repository

import (
	"context"
	"github-release-notifier/internal/dbtest"
	"testing"

	dbtx "github-release-notifier/internal/platform/db"

	"github.com/stretchr/testify/require"
)

func TestUserStore_CreateIfNotExists_CreatesUser(t *testing.T) {
	db := dbtest.SetupTestDB(t)
	store := NewUserStore(db)

	ctx := context.Background()
	email := dbtest.NewTestEmail()

	created, err := store.CreateIfNotExists(ctx, email)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, email, created.Email)
	require.False(t, created.CreatedAt.IsZero())
	require.False(t, created.UpdatedAt.IsZero())
}

func TestUserStore_CreateIfNotExists_WhenUserAlreadyExists_ReturnsExistingUser(t *testing.T) {
	db := dbtest.SetupTestDB(t)
	store := NewUserStore(db)

	ctx := context.Background()
	email := dbtest.NewTestEmail()

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
	db := dbtest.SetupTestDB(t)
	store := NewUserStore(db)

	tx := dbtest.BeginTestTx(t, db)
	created, err := store.CreateIfNotExists(dbtx.WithTransactionContext(context.Background(), tx), "tx-user@example.com")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var count int
	err = db.QueryRowContext(context.Background(), `select count(*) from users where id = $1`, created.ID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
