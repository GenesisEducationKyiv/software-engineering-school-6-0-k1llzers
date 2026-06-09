//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github-release-notifier/internal/platform/db/test"
	"testing"

	"github-release-notifier/internal/app/notifications"
	dbtx "github-release-notifier/internal/platform/db"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOutboxStore_CreateClaimAndMarkSent(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewOutboxStore(db)

	ctx := context.Background()
	recipientEmail, subject := newOutboxTestMessageIdentity("create-claim")
	err := store.Create(ctx, recipientEmail, notifications.Email{
		Subject:  subject,
		HTMLBody: "<b>hello</b>",
	})
	require.NoError(t, err)

	item := claimPendingTestMessage(t, ctx, store, recipientEmail, subject)
	require.Equal(t, recipientEmail, item.RecipientEmail)
	require.Equal(t, subject, item.Subject)
	require.Equal(t, 1, item.Attempts)

	err = store.MarkSent(ctx, item.ID)
	require.NoError(t, err)

	requireNoPendingOutboxRows(t, db, recipientEmail, subject)
}

func TestOutboxStore_Release_MakesMessageAvailableAgain(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewOutboxStore(db)

	ctx := context.Background()
	recipientEmail, subject := newOutboxTestMessageIdentity("release")
	err := store.Create(ctx, recipientEmail, notifications.Email{
		Subject:  subject,
		HTMLBody: "<b>hello</b>",
	})
	require.NoError(t, err)

	item := claimPendingTestMessage(t, ctx, store, recipientEmail, subject)

	err = store.Release(ctx, item.ID, "smtp failed")
	require.NoError(t, err)

	reclaimed := claimPendingTestMessage(t, ctx, store, recipientEmail, subject)
	require.Equal(t, item.ID, reclaimed.ID)
	require.Equal(t, 2, reclaimed.Attempts)
	require.NotNil(t, reclaimed.LastError)
	require.Equal(t, "smtp failed", *reclaimed.LastError)
}

func TestOutboxStore_ClaimNextPending_ReturnsOldestPendingMessage(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewOutboxStore(db)

	ctx := context.Background()
	firstRecipient, firstSubject := newOutboxTestMessageIdentity("first")
	secondRecipient, secondSubject := newOutboxTestMessageIdentity("second")
	err := store.Create(ctx, firstRecipient, notifications.Email{Subject: firstSubject, HTMLBody: "1"})
	require.NoError(t, err)
	err = store.Create(ctx, secondRecipient, notifications.Email{Subject: secondSubject, HTMLBody: "2"})
	require.NoError(t, err)

	first := claimPendingTestMessage(t, ctx, store, firstRecipient, firstSubject)
	second := claimPendingTestMessage(t, ctx, store, secondRecipient, secondSubject)

	require.Less(t, first.ID, second.ID)
	require.Equal(t, firstRecipient, first.RecipientEmail)
	require.Equal(t, secondRecipient, second.RecipientEmail)
}

func TestOutboxStore_Create_UsesTransaction(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewOutboxStore(db)

	tx := test.BeginTestTx(t, db)
	recipientEmail, subject := newOutboxTestMessageIdentity("tx")
	err := store.Create(dbtx.WithTransactionContext(context.Background(), tx), recipientEmail, notifications.Email{
		Subject:  subject,
		HTMLBody: "body",
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	item := claimPendingTestMessage(t, context.Background(), store, recipientEmail, subject)
	require.Equal(t, recipientEmail, item.RecipientEmail)
}

func newOutboxTestMessageIdentity(prefix string) (string, string) {
	id := uuid.NewString()
	return fmt.Sprintf("%s-%s@example.com", prefix, id), fmt.Sprintf("%s-%s", prefix, id)
}

func claimPendingTestMessage(t *testing.T, ctx context.Context, store *OutboxStore, recipientEmail string, subject string) notifications.Email {
	t.Helper()

	var foreignItems []notifications.Email

	for attempt := 0; attempt < 1000; attempt++ {
		item, err := store.ClaimNextPending(ctx, 60)
		require.NoError(t, err)
		if item.RecipientEmail == recipientEmail && item.Subject == subject {
			releaseClaimedOutboxItems(t, ctx, store, foreignItems)
			return item
		}

		foreignItems = append(foreignItems, item)
	}

	releaseClaimedOutboxItems(t, ctx, store, foreignItems)
	t.Fatalf("could not find pending outbox message for recipient %s and subject %s", recipientEmail, subject)
	return notifications.Email{}
}

func releaseClaimedOutboxItems(t *testing.T, ctx context.Context, store *OutboxStore, items []notifications.Email) {
	t.Helper()

	for _, item := range items {
		require.NoError(t, store.Release(ctx, item.ID, ""))
	}
}

func requireNoPendingOutboxRows(t *testing.T, db *sql.DB, recipientEmail string, subject string) {
	t.Helper()

	var pendingCount int
	err := db.QueryRowContext(
		context.Background(),
		`select count(*) from mail_outbox where recipient_email = $1 and subject = $2 and sent_at is null`,
		recipientEmail,
		subject,
	).Scan(&pendingCount)
	require.NoError(t, err)
	require.Zero(t, pendingCount)
}
