//go:build unit

package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github-release-notifier/internal/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type claimStoreStub struct {
	message       Message
	claimErr      error
	markErr       error
	releaseErr    error
	claimCalled   bool
	markCalledID  int64
	releaseCallID int64
}

func (s *claimStoreStub) ClaimNextPending(_ context.Context, _ int) (Message, error) {
	s.claimCalled = true
	if s.claimErr != nil {
		return Message{}, s.claimErr
	}

	return s.message, nil
}

func (s *claimStoreStub) MarkPublished(_ context.Context, id int64) error {
	s.markCalledID = id
	return s.markErr
}

func (s *claimStoreStub) Release(_ context.Context, id int64) error {
	s.releaseCallID = id
	return s.releaseErr
}

type publisherStub struct {
	err       error
	published Message
}

func (s *publisherStub) Publish(_ context.Context, message Message) error {
	s.published = message
	return s.err
}

func TestPublisherWorker_Run_PublishesAndMarksMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messageID := uuid.New()
	store := &claimStoreStub{
		message: Message{
			ID:        10,
			MessageID: messageID,
		},
	}
	publisher := &publisherStub{}
	worker := NewPublisherWorker(store, publisher)

	go worker.Run(ctx)

	require.Eventually(t, func() bool {
		return store.markCalledID == 10
	}, 250*time.Millisecond, 10*time.Millisecond)
	require.Equal(t, int64(10), publisher.published.ID)
	require.Zero(t, store.releaseCallID)
}

func TestPublisherWorker_Run_ReleasesMessageOnPublishError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messageID := uuid.New()
	store := &claimStoreStub{
		message: Message{
			ID:        10,
			MessageID: messageID,
		},
	}
	publisher := &publisherStub{err: errors.New("publish failed")}
	worker := NewPublisherWorker(store, publisher)

	go worker.Run(ctx)

	require.Eventually(t, func() bool {
		return store.releaseCallID == 10
	}, 250*time.Millisecond, 10*time.Millisecond)
	require.Zero(t, store.markCalledID)
}

func TestPublisherWorker_Run_WaitsWhenOutboxIsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &claimStoreStub{claimErr: shared.ErrNotFound}
	worker := NewPublisherWorker(store, &publisherStub{})

	go worker.Run(ctx)

	require.Eventually(t, func() bool {
		return store.claimCalled
	}, 250*time.Millisecond, 10*time.Millisecond)
	require.Zero(t, store.markCalledID)
	require.Zero(t, store.releaseCallID)
}
