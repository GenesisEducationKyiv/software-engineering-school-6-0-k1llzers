//go:build unit

package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github-release-notifier/internal/domain"
	appmetrics "github-release-notifier/internal/metrics"

	"github.com/stretchr/testify/require"
)

type outboxStoreStub struct {
	claimResults []Email
	claimErrs    []error
	claimCalls   int
	markSentID   int64
	releaseID    int64
	releaseError string
	releaseErr   error
	markSentErr  error
}

func (s *outboxStoreStub) ClaimNextPending(_ context.Context, _ int) (Email, error) {
	call := s.claimCalls
	s.claimCalls++

	if call < len(s.claimErrs) && s.claimErrs[call] != nil {
		return Email{}, s.claimErrs[call]
	}
	if call < len(s.claimResults) {
		return s.claimResults[call], nil
	}

	return Email{}, domain.ErrNotFound
}

func (s *outboxStoreStub) MarkSent(_ context.Context, id int64) error {
	s.markSentID = id
	return s.markSentErr
}

func (s *outboxStoreStub) Release(_ context.Context, id int64, lastError string) error {
	s.releaseID = id
	s.releaseError = lastError
	return s.releaseErr
}

type senderStub struct {
	err          error
	to           string
	email        RenderedEmail
	calls        int
	cancelOnSend context.CancelFunc
}

func (s *senderStub) Deliver(_ context.Context, to string, email RenderedEmail) error {
	s.calls++
	s.to = to
	s.email = email
	if s.cancelOnSend != nil {
		s.cancelOnSend()
	}
	return s.err
}

func TestOutboxDispatcher_Run_SendsAndMarksSent(t *testing.T) {
	store := &outboxStoreStub{
		claimResults: []Email{
			{
				ID:             10,
				RecipientEmail: "user@example.com",
				Subject:        "subject",
				HTMLBody:       "<p>body</p>",
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	sender := &senderStub{cancelOnSend: cancel}
	dispatcher := NewOutboxDispatcher(store, sender, newTestMetrics(t))

	dispatcher.Run(ctx)

	require.Equal(t, 1, sender.calls)
	require.Equal(t, "user@example.com", sender.to)
	require.Equal(t, RenderedEmail{Subject: "subject", HTMLBody: "<p>body</p>"}, sender.email)
	require.Equal(t, int64(10), store.markSentID)
	require.Zero(t, store.releaseID)
}

func TestOutboxDispatcher_Run_ReleasesEmailWhenSendFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &outboxStoreStub{
		claimResults: []Email{
			{
				ID:             20,
				RecipientEmail: "user@example.com",
				Subject:        "subject",
				HTMLBody:       "<p>body</p>",
			},
		},
		claimErrs: []error{nil, domain.ErrNotFound},
	}
	sender := &senderStub{
		err: errors.New("smtp failed"),
		cancelOnSend: func() {
			time.AfterFunc(10*time.Millisecond, cancel)
		},
	}
	dispatcher := NewOutboxDispatcher(store, sender, newTestMetrics(t))

	dispatcher.Run(ctx)

	require.Equal(t, 1, sender.calls)
	require.Equal(t, int64(20), store.releaseID)
	require.Equal(t, "smtp failed", store.releaseError)
	require.Zero(t, store.markSentID)
}

func TestOutboxDispatcher_Run_StopsImmediatelyWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := &outboxStoreStub{}
	sender := &senderStub{}
	dispatcher := NewOutboxDispatcher(store, sender, newTestMetrics(t))

	dispatcher.Run(ctx)

	require.Zero(t, store.claimCalls)
	require.Zero(t, sender.calls)
}

func newTestMetrics(t *testing.T) *appmetrics.Metrics {
	t.Helper()

	metricSet, err := appmetrics.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, metricSet.Shutdown(context.Background()))
	})

	return metricSet
}
