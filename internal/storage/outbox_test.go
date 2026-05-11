package storage

import (
	"context"
	"testing"

	dbtx "github-release-notifier/internal/db"
	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/outbox"

	"github.com/stretchr/testify/require"
)

func TestOutboxStore_CreateClaimAndMarkSent(t *testing.T) {
	db := setupTestDB(t)
	store := NewOutboxStore(db)

	ctx := context.Background()
	err := store.Create(ctx, "test@example.com", outbox.Email{
		Subject:  "subject",
		HTMLBody: "<b>hello</b>",
	})
	require.NoError(t, err)

	item, err := store.ClaimNextPending(ctx, 60)
	require.NoError(t, err)
	require.Equal(t, "test@example.com", item.RecipientEmail)
	require.Equal(t, "subject", item.Subject)
	require.Equal(t, 1, item.Attempts)

	err = store.MarkSent(ctx, item.ID)
	require.NoError(t, err)

	_, err = store.ClaimNextPending(ctx, 60)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestOutboxStore_Release_MakesMessageAvailableAgain(t *testing.T) {
	db := setupTestDB(t)
	store := NewOutboxStore(db)

	ctx := context.Background()
	err := store.Create(ctx, "test@example.com", outbox.Email{
		Subject:  "subject",
		HTMLBody: "<b>hello</b>",
	})
	require.NoError(t, err)

	item, err := store.ClaimNextPending(ctx, 60)
	require.NoError(t, err)

	err = store.Release(ctx, item.ID, "smtp failed")
	require.NoError(t, err)

	reclaimed, err := store.ClaimNextPending(ctx, 60)
	require.NoError(t, err)
	require.Equal(t, item.ID, reclaimed.ID)
	require.Equal(t, 2, reclaimed.Attempts)
	require.NotNil(t, reclaimed.LastError)
	require.Equal(t, "smtp failed", *reclaimed.LastError)
}

func TestOutboxStore_ClaimNextPending_ReturnsOldestPendingMessage(t *testing.T) {
	db := setupTestDB(t)
	store := NewOutboxStore(db)

	ctx := context.Background()
	err := store.Create(ctx, "first@example.com", outbox.Email{Subject: "first", HTMLBody: "1"})
	require.NoError(t, err)
	err = store.Create(ctx, "second@example.com", outbox.Email{Subject: "second", HTMLBody: "2"})
	require.NoError(t, err)

	first, err := store.ClaimNextPending(ctx, 60)
	require.NoError(t, err)
	second, err := store.ClaimNextPending(ctx, 60)
	require.NoError(t, err)

	require.Less(t, first.ID, second.ID)
	require.Equal(t, "first@example.com", first.RecipientEmail)
	require.Equal(t, "second@example.com", second.RecipientEmail)
}

func TestOutboxStore_Create_UsesTransaction(t *testing.T) {
	db := setupTestDB(t)
	store := NewOutboxStore(db)

	tx := beginTestTx(t, db)
	err := store.Create(dbtx.WithTransactionContext(context.Background(), tx), "tx@example.com", outbox.Email{
		Subject:  "subject",
		HTMLBody: "body",
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	item, err := store.ClaimNextPending(context.Background(), 60)
	require.NoError(t, err)
	require.Equal(t, "tx@example.com", item.RecipientEmail)
}
