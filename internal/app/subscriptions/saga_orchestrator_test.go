//go:build unit

package subscriptions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github-release-notifier/internal/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type sagaStoreStub struct {
	saga                  Saga
	claimErr              error
	quotaCommittedID      uuid.UUID
	completedID           uuid.UUID
	rejectedID            uuid.UUID
	rejectedReason        string
	deadID                uuid.UUID
	deadCause             error
	failedID              uuid.UUID
	failedSagaStatus      SagaStatus
	failedCause           error
	failedNextRetryAt     time.Time
	failedMaxAttempts     int
	failedStatus          SagaStatus
	compensatingID        uuid.UUID
	compensatingCause     error
	compensationFailedID  uuid.UUID
	compensationFailedErr error
	compensatedID         uuid.UUID
	markFailedErr         error
	markCompleteErr       error
}

func (s *sagaStoreStub) Get(_ context.Context, sagaID uuid.UUID) (Saga, error) {
	s.saga.ID = sagaID
	return s.saga, nil
}

func (s *sagaStoreStub) ClaimByID(_ context.Context, sagaID uuid.UUID, _ int) (Saga, error) {
	if s.claimErr != nil {
		return Saga{}, s.claimErr
	}
	s.saga.ID = sagaID
	return s.saga, nil
}

func (s *sagaStoreStub) ClaimNextPending(context.Context, int) (Saga, error) {
	if s.claimErr != nil {
		return Saga{}, s.claimErr
	}
	return s.saga, nil
}

func (s *sagaStoreStub) MarkCompleted(_ context.Context, sagaID uuid.UUID) error {
	s.completedID = sagaID
	return s.markCompleteErr
}

func (s *sagaStoreStub) MarkRejected(_ context.Context, saga Saga, reason string) error {
	s.rejectedID = saga.ID
	s.rejectedReason = reason
	s.saga.Status = SagaStatusRejected
	return nil
}

func (s *sagaStoreStub) MarkDead(_ context.Context, saga Saga, cause error) error {
	s.deadID = saga.ID
	s.deadCause = cause
	s.saga.Status = SagaStatusDead
	return nil
}

func (s *sagaStoreStub) MarkQuotaCommitted(_ context.Context, sagaID uuid.UUID) error {
	s.quotaCommittedID = sagaID
	s.saga.Status = SagaStatusQuotaCommitted
	return nil
}

func (s *sagaStoreStub) MarkFailed(_ context.Context, saga Saga, cause error, nextRetryAt time.Time, maxAttempts int) (SagaStatus, error) {
	s.failedID = saga.ID
	s.failedSagaStatus = saga.Status
	s.failedCause = cause
	s.failedNextRetryAt = nextRetryAt
	s.failedMaxAttempts = maxAttempts
	if s.failedStatus == "" {
		s.failedStatus = SagaStatusFailed
	}
	return s.failedStatus, s.markFailedErr
}

func (s *sagaStoreStub) MarkCompensating(_ context.Context, saga Saga, cause error) error {
	s.compensatingID = saga.ID
	s.compensatingCause = cause
	return nil
}

func (s *sagaStoreStub) MarkCompensationFailed(_ context.Context, saga Saga, cause error, _ time.Time, _ int) error {
	s.compensationFailedID = saga.ID
	s.compensationFailedErr = cause
	return nil
}

func (s *sagaStoreStub) MarkCompensated(_ context.Context, sagaID uuid.UUID) error {
	s.compensatedID = sagaID
	return nil
}

type sagaSubscriptionStoreStub struct {
	details    SagaSubscriptionDetails
	detailsErr error
	deleteByID int64
	deleteErr  error
}

func (s *sagaSubscriptionStoreStub) GetSubscriptionDetailsForSaga(context.Context, int64) (SagaSubscriptionDetails, error) {
	if s.detailsErr != nil {
		return SagaSubscriptionDetails{}, s.detailsErr
	}
	return s.details, nil
}

func (s *sagaSubscriptionStoreStub) DeleteByID(_ context.Context, subscriptionID int64) error {
	s.deleteByID = subscriptionID
	return s.deleteErr
}

type sagaQuotaClientStub struct {
	reserveResult  QuotaReserveResult
	reserveErr     error
	commitErr      error
	releaseErr     error
	reservedSagaID uuid.UUID
	reservedID     int64
	reservedEmail  string
	committedSaga  uuid.UUID
	committedID    int64
	releasedSaga   uuid.UUID
	releasedID     int64
}

func (c *sagaQuotaClientStub) ReserveSubscriptionSlot(_ context.Context, sagaID uuid.UUID, subscriptionID int64, email string) (QuotaReserveResult, error) {
	c.reservedSagaID = sagaID
	c.reservedID = subscriptionID
	c.reservedEmail = email
	if c.reserveErr != nil {
		return QuotaReserveResult{}, c.reserveErr
	}
	return c.reserveResult, nil
}

func (c *sagaQuotaClientStub) CommitSubscriptionSlot(_ context.Context, sagaID uuid.UUID, subscriptionID int64) error {
	c.committedSaga = sagaID
	c.committedID = subscriptionID
	return c.commitErr
}

func (c *sagaQuotaClientStub) ReleaseSubscriptionSlot(_ context.Context, sagaID uuid.UUID, subscriptionID int64) error {
	c.releasedSaga = sagaID
	c.releasedID = subscriptionID
	return c.releaseErr
}

func TestSagaOrchestrator_ProcessSubscribe_CompletesSagaAndQueuesEmail(t *testing.T) {
	sagaID := uuid.New()
	sagas := &sagaStoreStub{saga: Saga{ID: sagaID, SubscriptionID: 10, Operation: SagaOperationSubscribe}}
	subscriptions := &sagaSubscriptionStoreStub{details: SagaSubscriptionDetails{
		Subscription: Subscription{
			ID:                10,
			ConfirmationToken: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
		Email:              "test@example.com",
		RepositoryFullName: "gin-gonic/gin",
	}}
	quota := &sagaQuotaClientStub{reserveResult: QuotaReserveResult{Reserved: true}}
	mail := &confirmationSenderStub{}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, mail)

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.NoError(t, err)
	require.Equal(t, sagaID, quota.reservedSagaID)
	require.Equal(t, int64(10), quota.reservedID)
	require.Equal(t, "test@example.com", quota.reservedEmail)
	require.Equal(t, sagaID, quota.committedSaga)
	require.Equal(t, int64(10), quota.committedID)
	require.Equal(t, sagaID, sagas.quotaCommittedID)
	require.Equal(t, sagaID, sagas.completedID)
	require.Equal(t, "test@example.com", mail.recipientEmail)
	require.Equal(t, uuid.Nil, sagas.failedID)
}

func TestSagaOrchestrator_ProcessSubscribe_DeletesPendingWhenQuotaRejected(t *testing.T) {
	sagaID := uuid.New()
	sagas := &sagaStoreStub{saga: Saga{ID: sagaID, SubscriptionID: 10, Operation: SagaOperationSubscribe}}
	subscriptions := &sagaSubscriptionStoreStub{details: SagaSubscriptionDetails{Subscription: Subscription{ID: 10}, Email: "test@example.com"}}
	quota := &sagaQuotaClientStub{reserveResult: QuotaReserveResult{Reserved: false, RejectionReason: "limit exceeded"}}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, &confirmationSenderStub{})

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.ErrorIs(t, err, ErrQuotaRejected)
	require.Equal(t, int64(10), subscriptions.deleteByID)
	require.Equal(t, sagaID, sagas.rejectedID)
	require.Equal(t, "limit exceeded", sagas.rejectedReason)
	require.Equal(t, uuid.Nil, sagas.completedID)
	require.Equal(t, uuid.Nil, sagas.failedID)
}

func TestSagaOrchestrator_ProcessSubscribe_MarksDeadForPreCommitFailure(t *testing.T) {
	sagaID := uuid.New()
	expectedErr := errors.New("quota unavailable")
	sagas := &sagaStoreStub{saga: Saga{ID: sagaID, SubscriptionID: 10, Operation: SagaOperationSubscribe}}
	subscriptions := &sagaSubscriptionStoreStub{details: SagaSubscriptionDetails{Subscription: Subscription{ID: 10}, Email: "test@example.com"}}
	quota := &sagaQuotaClientStub{reserveErr: expectedErr}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, &confirmationSenderStub{})

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, int64(10), subscriptions.deleteByID)
	require.Equal(t, sagaID, sagas.deadID)
	require.ErrorContains(t, sagas.deadCause, "reserve quota slot")
	require.Equal(t, uuid.Nil, sagas.failedID)
	require.Equal(t, uuid.Nil, sagas.completedID)
}

func TestSagaOrchestrator_ProcessSubscribe_RetriesLocalStepAfterQuotaCommitted(t *testing.T) {
	sagaID := uuid.New()
	sagas := &sagaStoreStub{saga: Saga{ID: sagaID, SubscriptionID: 10, Operation: SagaOperationSubscribe, Status: SagaStatusQuotaCommitted}}
	subscriptions := &sagaSubscriptionStoreStub{details: SagaSubscriptionDetails{
		Subscription: Subscription{
			ID:                10,
			ConfirmationToken: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
		Email:              "test@example.com",
		RepositoryFullName: "gin-gonic/gin",
	}}
	quota := &sagaQuotaClientStub{}
	mail := &confirmationSenderStub{}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, mail)

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.NoError(t, err)
	require.Equal(t, uuid.Nil, quota.reservedSagaID)
	require.Equal(t, uuid.Nil, quota.committedSaga)
	require.Zero(t, quota.committedID)
	require.Equal(t, "test@example.com", mail.recipientEmail)
	require.Equal(t, sagaID, sagas.completedID)
}

func TestSagaOrchestrator_ProcessSubscribe_KeepsQuotaCommittedWhenLocalStepFails(t *testing.T) {
	sagaID := uuid.New()
	expectedErr := errors.New("outbox unavailable")
	sagas := &sagaStoreStub{saga: Saga{ID: sagaID, SubscriptionID: 10, Operation: SagaOperationSubscribe, Status: SagaStatusRunning}}
	subscriptions := &sagaSubscriptionStoreStub{details: SagaSubscriptionDetails{
		Subscription: Subscription{
			ID:                10,
			ConfirmationToken: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CancellationToken: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
		Email:              "test@example.com",
		RepositoryFullName: "gin-gonic/gin",
	}}
	quota := &sagaQuotaClientStub{reserveResult: QuotaReserveResult{Reserved: true}}
	mail := &confirmationSenderStub{err: expectedErr}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, mail)

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, sagaID, sagas.quotaCommittedID)
	require.Equal(t, sagaID, sagas.failedID)
	require.Equal(t, SagaStatusQuotaCommitted, sagas.failedSagaStatus)
	require.Equal(t, uuid.Nil, sagas.compensatingID)
	require.Equal(t, uuid.Nil, sagas.completedID)
}

func TestSagaOrchestrator_ProcessSubscribe_CompensatesTerminalQuotaCommittedFailure(t *testing.T) {
	sagaID := uuid.New()
	expectedErr := errors.New("outbox unavailable")
	sagas := &sagaStoreStub{saga: Saga{
		ID:             sagaID,
		SubscriptionID: 10,
		Operation:      SagaOperationSubscribe,
		Status:         SagaStatusQuotaCommitted,
		Attempts:       defaultSagaMaxAttempts - 1,
	}}
	subscriptions := &sagaSubscriptionStoreStub{details: SagaSubscriptionDetails{
		Subscription: Subscription{ID: 10},
		Email:        "test@example.com",
	}}
	quota := &sagaQuotaClientStub{}
	mail := &confirmationSenderStub{err: expectedErr}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, mail)

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, sagaID, sagas.compensatingID)
	require.Equal(t, sagaID, quota.releasedSaga)
	require.Equal(t, int64(10), quota.releasedID)
	require.Equal(t, int64(10), subscriptions.deleteByID)
	require.Equal(t, sagaID, sagas.compensatedID)
	require.Equal(t, uuid.Nil, sagas.failedID)
}

func TestSagaOrchestrator_ProcessUnsubscribe_ReleasesQuotaAndDeletesSubscription(t *testing.T) {
	sagaID := uuid.New()
	sagas := &sagaStoreStub{saga: Saga{ID: sagaID, SubscriptionID: 10, Operation: SagaOperationUnsubscribe}}
	subscriptions := &sagaSubscriptionStoreStub{}
	quota := &sagaQuotaClientStub{}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, subscriptions, quota, &confirmationSenderStub{})

	err := orchestrator.ProcessByID(context.Background(), sagaID)

	require.NoError(t, err)
	require.Equal(t, sagaID, quota.releasedSaga)
	require.Equal(t, int64(10), quota.releasedID)
	require.Equal(t, int64(10), subscriptions.deleteByID)
	require.Equal(t, sagaID, sagas.completedID)
}

func TestSagaRetryWorker_StopsWhenNoPendingSaga(t *testing.T) {
	sagas := &sagaStoreStub{claimErr: shared.ErrNotFound}
	orchestrator := NewSagaOrchestrator(&transactionManagerStub{}, sagas, &sagaSubscriptionStoreStub{}, &sagaQuotaClientStub{}, &confirmationSenderStub{})
	worker := NewSagaRetryWorker(orchestrator)
	worker.emptyDelay = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker.Run(ctx)
}
