//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	dbtest "github-release-notifier/internal/platform/db/test"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMessageInboxStore_ClaimForProcessing_CreatesInboxMessage(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := NewMessageInboxStore(db)

	messageID := uuid.New()
	payloadJSON, err := json.Marshal(map[string]string{"value": uuid.NewString()})
	require.NoError(t, err)

	claimed, err := store.ClaimForProcessing(context.Background(), messageID, "subscription.confirmation.requested", payloadJSON)
	require.NoError(t, err)
	require.True(t, claimed)

	var count int
	err = db.QueryRowContext(context.Background(), `select count(*) from message_inbox where message_id = $1`, messageID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var processingStartedAt sql.NullTime
	err = db.QueryRowContext(context.Background(), `select processing_started_at from message_inbox where message_id = $1`, messageID).Scan(&processingStartedAt)
	require.NoError(t, err)
	require.True(t, processingStartedAt.Valid)
}

func TestMessageInboxStore_ClaimForProcessing_IgnoresDuplicateInProgressMessage(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := NewMessageInboxStore(db)

	messageID := uuid.New()
	payloadJSON, err := json.Marshal(map[string]string{"value": uuid.NewString()})
	require.NoError(t, err)

	claimed, err := store.ClaimForProcessing(context.Background(), messageID, "subscription.confirmation.requested", payloadJSON)
	require.NoError(t, err)
	require.True(t, claimed)

	claimed, err = store.ClaimForProcessing(context.Background(), messageID, "subscription.confirmation.requested", payloadJSON)
	require.NoError(t, err)
	require.False(t, claimed)
}

func TestMessageInboxStore_MarkProcessed_MarksMessageAsProcessed(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := NewMessageInboxStore(db)

	messageID := uuid.New()
	payloadJSON, err := json.Marshal(map[string]string{"value": uuid.NewString()})
	require.NoError(t, err)

	claimed, err := store.ClaimForProcessing(context.Background(), messageID, "subscription.confirmation.requested", payloadJSON)
	require.NoError(t, err)
	require.True(t, claimed)

	err = store.MarkProcessed(context.Background(), messageID)
	require.NoError(t, err)

	var processingStartedAt sql.NullTime
	var processedAt sql.NullTime
	err = db.QueryRowContext(
		context.Background(),
		`select processing_started_at, processed_at from message_inbox where message_id = $1`,
		messageID,
	).Scan(&processingStartedAt, &processedAt)
	require.NoError(t, err)
	require.False(t, processingStartedAt.Valid)
	require.True(t, processedAt.Valid)
}

func TestMessageInboxStore_ReleaseProcessing_MakesMessageClaimableAgain(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := NewMessageInboxStore(db)

	messageID := uuid.New()
	payloadJSON, err := json.Marshal(map[string]string{"value": uuid.NewString()})
	require.NoError(t, err)

	claimed, err := store.ClaimForProcessing(context.Background(), messageID, "subscription.confirmation.requested", payloadJSON)
	require.NoError(t, err)
	require.True(t, claimed)

	err = store.ReleaseProcessing(context.Background(), messageID)
	require.NoError(t, err)

	claimed, err = store.ClaimForProcessing(context.Background(), messageID, "subscription.confirmation.requested", payloadJSON)
	require.NoError(t, err)
	require.True(t, claimed)
}
