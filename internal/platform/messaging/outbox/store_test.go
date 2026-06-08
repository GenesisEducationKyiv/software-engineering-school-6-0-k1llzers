//go:build integration

package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	dbtx "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/db/test"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestStore_CreateClaimAndMarkPublished(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewStore(db)

	ctx := context.Background()
	message := newTestMessage("create-claim")
	err := store.Create(ctx, message)
	require.NoError(t, err)

	item := claimPendingTestMessage(t, ctx, store, message.MessageID)
	require.Equal(t, message.MessageID, item.MessageID)
	require.Equal(t, message.MessageType, item.MessageType)

	err = store.MarkPublished(ctx, item.ID)
	require.NoError(t, err)

	requireNoPendingOutboxRows(t, db, message.MessageID)
}

func TestStore_Release_MakesMessageAvailableAgain(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewStore(db)

	ctx := context.Background()
	message := newTestMessage("release")
	err := store.Create(ctx, message)
	require.NoError(t, err)

	item := claimPendingTestMessage(t, ctx, store, message.MessageID)

	err = store.Release(ctx, item.ID)
	require.NoError(t, err)

	reclaimed := claimPendingTestMessage(t, ctx, store, message.MessageID)
	require.Equal(t, item.ID, reclaimed.ID)
}

func TestStore_Create_UsesTransaction(t *testing.T) {
	db := test.SetupTestDB(t)
	store := NewStore(db)

	tx := test.BeginTestTx(t, db)
	message := newTestMessage("tx")
	err := store.Create(dbtx.WithTransactionContext(context.Background(), tx), message)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	item := claimPendingTestMessage(t, context.Background(), store, message.MessageID)
	require.Equal(t, message.MessageID, item.MessageID)
}

func newTestMessage(prefix string) Message {
	id := uuid.New()
	payloadJSON, _ := json.Marshal(map[string]string{
		"kind": prefix,
		"id":   id.String(),
	})

	return Message{
		MessageID:   id,
		MessageType: fmt.Sprintf("test.%s.requested", prefix),
		PayloadJSON: payloadJSON,
	}
}

func claimPendingTestMessage(t *testing.T, ctx context.Context, store *Store, messageID uuid.UUID) Message {
	t.Helper()

	var foreignItems []Message

	for attempt := 0; attempt < 1000; attempt++ {
		item, err := store.ClaimNextPending(ctx, 60)
		require.NoError(t, err)
		if item.MessageID == messageID {
			releaseClaimedOutboxItems(t, ctx, store, foreignItems)
			return item
		}

		foreignItems = append(foreignItems, item)
	}

	releaseClaimedOutboxItems(t, ctx, store, foreignItems)
	t.Fatalf("could not find pending integration outbox message %s", messageID)
	return Message{}
}

func releaseClaimedOutboxItems(t *testing.T, ctx context.Context, store *Store, items []Message) {
	t.Helper()

	for _, item := range items {
		require.NoError(t, store.Release(ctx, item.ID))
	}
}

func requireNoPendingOutboxRows(t *testing.T, db *sql.DB, messageID uuid.UUID) {
	t.Helper()

	var pendingCount int
	err := db.QueryRowContext(
		context.Background(),
		`select count(*) from integration_outbox where message_id = $1 and published_at is null`,
		messageID,
	).Scan(&pendingCount)
	require.NoError(t, err)
	require.Zero(t, pendingCount)
}
