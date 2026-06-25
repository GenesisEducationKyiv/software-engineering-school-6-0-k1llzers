//go:build integration

package notifications

import (
	"context"
	"database/sql"
	"testing"

	notificationsrepo "github-release-notifier/internal/notification/notifications/repository"
	dbtest "github-release-notifier/internal/platform/db/test"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type integrationDeliveryStub struct {
	calls int
	err   error
}

func (s *integrationDeliveryStub) Deliver(_ context.Context, _ notificationcontracts.Envelope) error {
	s.calls++
	return s.err
}

func TestMessageInboxHandler_Handle_MarksMessageProcessed(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := notificationsrepo.NewMessageInboxStore(db)
	delivery := &integrationDeliveryStub{}
	handler := NewMessageInboxHandler(store, delivery)
	messageID := uuid.New()
	message, err := notificationcontracts.NewSubscriptionConfirmationRequestedMessage(
		messageID,
		notificationcontracts.SubscriptionConfirmationRequested{
			RecipientEmail:     dbtest.NewTestEmail(),
			RepositoryFullName: "gin-gonic/gin",
			ConfirmationToken:  uuid.New(),
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.NoError(t, err)
	require.Equal(t, 1, delivery.calls)

	assertMessageInboxState(t, db, messageID, false, true)
}

func TestMessageInboxHandler_Handle_DuplicateMessageSkipsDelivery(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := notificationsrepo.NewMessageInboxStore(db)
	delivery := &integrationDeliveryStub{}
	handler := NewMessageInboxHandler(store, delivery)
	messageID := uuid.New()
	message, err := notificationcontracts.NewReleaseNotificationRequestedMessage(
		messageID,
		notificationcontracts.ReleaseNotificationRequested{
			RecipientEmail:     dbtest.NewTestEmail(),
			RepositoryFullName: "gin-gonic/gin",
			TagName:            "v1.11.0",
			ReleaseURL:         "https://example.com/release",
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	require.NoError(t, handler.Handle(context.Background(), message))
	require.NoError(t, handler.Handle(context.Background(), message))
	require.Equal(t, 1, delivery.calls)

	assertMessageInboxState(t, db, messageID, false, true)
}

func TestMessageInboxHandler_Handle_DeliveryErrorReleasesProcessing(t *testing.T) {
	db := dbtest.SetupNotificationTestDB(t)
	store := notificationsrepo.NewMessageInboxStore(db)
	delivery := &integrationDeliveryStub{err: assertAnError{}}
	handler := NewMessageInboxHandler(store, delivery)
	messageID := uuid.New()
	message, err := notificationcontracts.NewReleaseNotificationRequestedMessage(
		messageID,
		notificationcontracts.ReleaseNotificationRequested{
			RecipientEmail:     dbtest.NewTestEmail(),
			RepositoryFullName: "gin-gonic/gin",
			TagName:            "v1.11.0",
			ReleaseURL:         "https://example.com/release",
			CancellationToken:  uuid.New(),
		},
	)
	require.NoError(t, err)

	err = handler.Handle(context.Background(), message)
	require.Error(t, err)
	require.Equal(t, 1, delivery.calls)

	assertMessageInboxState(t, db, messageID, false, false)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "deliver failed" }

func assertMessageInboxState(t *testing.T, db *sql.DB, messageID uuid.UUID, processingExpected bool, processedExpected bool) {
	t.Helper()

	var processingStartedAt sql.NullTime
	var processedAt sql.NullTime
	err := db.QueryRowContext(
		context.Background(),
		`select processing_started_at, processed_at from message_inbox where message_id = $1`,
		messageID,
	).Scan(&processingStartedAt, &processedAt)
	require.NoError(t, err)
	require.Equal(t, processingExpected, processingStartedAt.Valid)
	require.Equal(t, processedExpected, processedAt.Valid)
}
