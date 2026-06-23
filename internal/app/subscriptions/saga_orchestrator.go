package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github-release-notifier/internal/shared"

	"github.com/google/uuid"
)

const (
	defaultSagaProcessingTimeoutSeconds = 60
	defaultSagaMaxAttempts              = 5
	defaultSagaRetryBaseDelay           = 5 * time.Second
)

type sagaStore interface {
	Get(ctx context.Context, sagaID uuid.UUID) (Saga, error)
	ClaimByID(ctx context.Context, sagaID uuid.UUID, processingTimeoutSeconds int) (Saga, error)
	ClaimNextPending(ctx context.Context, processingTimeoutSeconds int) (Saga, error)
	MarkQuotaCommitted(ctx context.Context, sagaID uuid.UUID) error
	MarkCompleted(ctx context.Context, sagaID uuid.UUID) error
	MarkRejected(ctx context.Context, saga Saga, reason string) error
	MarkDead(ctx context.Context, saga Saga, cause error) error
	MarkFailed(ctx context.Context, saga Saga, cause error, nextRetryAt time.Time, maxAttempts int) (SagaStatus, error)
	MarkCompensating(ctx context.Context, saga Saga, cause error) error
	MarkCompensationFailed(ctx context.Context, saga Saga, cause error, nextRetryAt time.Time, maxAttempts int) error
	MarkCompensated(ctx context.Context, sagaID uuid.UUID) error
}

type sagaSubscriptionStore interface {
	GetSubscriptionDetailsForSaga(ctx context.Context, subscriptionID int64) (SagaSubscriptionDetails, error)
	DeleteByID(ctx context.Context, subscriptionID int64) error
}

type sagaQuotaClient interface {
	ReserveSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64, email string) (QuotaReserveResult, error)
	CommitSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64) error
	ReleaseSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64) error
}

type QuotaReserveResult struct {
	Reserved        bool
	RejectionReason string
}

type subscribeProgressError struct {
	err            error
	quotaReserved  bool
	quotaCommitted bool
}

func (e *subscribeProgressError) Error() string {
	return e.err.Error()
}

func (e *subscribeProgressError) Unwrap() error {
	return e.err
}

type SagaOrchestrator struct {
	txManager     txManager
	sagas         sagaStore
	subscriptions sagaSubscriptionStore
	quota         sagaQuotaClient
	mailQueue     confirmationQueue
}

func NewSagaOrchestrator(
	txManager txManager,
	sagas sagaStore,
	subscriptions sagaSubscriptionStore,
	quota sagaQuotaClient,
	mailQueue confirmationQueue,
) *SagaOrchestrator {
	return &SagaOrchestrator{
		txManager:     txManager,
		sagas:         sagas,
		subscriptions: subscriptions,
		quota:         quota,
		mailQueue:     mailQueue,
	}
}

func (o *SagaOrchestrator) ProcessByID(ctx context.Context, sagaID uuid.UUID) error {
	saga, err := o.sagas.ClaimByID(ctx, sagaID, defaultSagaProcessingTimeoutSeconds)
	if err != nil {
		return err
	}

	return o.processClaimed(ctx, saga)
}

func (o *SagaOrchestrator) ProcessNext(ctx context.Context) error {
	saga, err := o.sagas.ClaimNextPending(ctx, defaultSagaProcessingTimeoutSeconds)
	if err != nil {
		return err
	}

	return o.processClaimed(ctx, saga)
}

func (o *SagaOrchestrator) processClaimed(ctx context.Context, saga Saga) error {
	if saga.Status == SagaStatusCompensating {
		return o.processCompensation(ctx, saga)
	}

	var err error
	switch saga.Operation {
	case SagaOperationSubscribe:
		err = o.processSubscribe(ctx, saga)
	case SagaOperationUnsubscribe:
		err = o.processUnsubscribe(ctx, saga)
	default:
		err = fmt.Errorf("unsupported subscription saga operation: %s", saga.Operation)
	}

	if err == nil {
		return nil
	}
	if errors.Is(err, ErrQuotaRejected) {
		return err
	}

	currentSaga := saga
	if refreshed, refreshErr := o.sagas.Get(ctx, saga.ID); refreshErr == nil {
		currentSaga = refreshed
	} else {
		slog.WarnContext(ctx, "subscription saga status refresh failed", "saga_id", saga.ID.String(), "error", refreshErr)
	}

	var progressErr *subscribeProgressError
	if errors.As(err, &progressErr) && progressErr.quotaCommitted {
		return err
	}

	if errors.As(err, &progressErr) && progressErr.quotaReserved {
		o.releaseReservedQuotaBestEffort(ctx, currentSaga)
	}

	if o.shouldTerminallyFailSubscribeBeforeQuotaCommit(currentSaga) {
		if markErr := o.markSubscribeDead(ctx, currentSaga, err); markErr != nil {
			return fmt.Errorf("%w; mark subscribe saga dead: %v", err, markErr)
		}

		return err
	}

	if o.shouldCompensateSubscribe(currentSaga) && currentSaga.Attempts+1 >= defaultSagaMaxAttempts {
		if markErr := o.sagas.MarkCompensating(ctx, currentSaga, err); markErr != nil {
			return fmt.Errorf("%w; mark saga compensating: %v", err, markErr)
		}

		if compensationErr := o.compensateSubscribe(ctx, currentSaga); compensationErr != nil {
			if markErr := o.sagas.MarkCompensationFailed(ctx, currentSaga, compensationErr, nextSagaRetryAt(currentSaga.Attempts+1), defaultSagaMaxAttempts); markErr != nil {
				return fmt.Errorf("%w; compensate subscribe: %v; mark compensation failed: %v", err, compensationErr, markErr)
			}

			return fmt.Errorf("%w; compensate subscribe: %v", err, compensationErr)
		}

		return err
	}

	nextRetryAt := nextSagaRetryAt(currentSaga.Attempts + 1)
	if _, markErr := o.sagas.MarkFailed(ctx, currentSaga, err, nextRetryAt, defaultSagaMaxAttempts); markErr != nil {
		return fmt.Errorf("%w; mark saga failed: %v", err, markErr)
	}

	return err
}

func (o *SagaOrchestrator) processSubscribe(ctx context.Context, saga Saga) error {
	var details SagaSubscriptionDetails

	if saga.Status != SagaStatusQuotaCommitted {
		loadedDetails, err := o.subscriptions.GetSubscriptionDetailsForSaga(ctx, saga.SubscriptionID)
		if err != nil {
			return err
		}
		details = loadedDetails

		reserveResult, err := o.quota.ReserveSubscriptionSlot(ctx, saga.ID, saga.SubscriptionID, details.Email)
		if err != nil {
			return fmt.Errorf("reserve quota slot: %w", err)
		}

		if !reserveResult.Reserved {
			return o.handleQuotaRejected(ctx, saga, reserveResult.RejectionReason)
		}

		if err := o.quota.CommitSubscriptionSlot(ctx, saga.ID, saga.SubscriptionID); err != nil {
			return &subscribeProgressError{
				err:           fmt.Errorf("commit quota slot: %w", err),
				quotaReserved: true,
			}
		}

		if err := o.sagas.MarkQuotaCommitted(ctx, saga.ID); err != nil {
			return &subscribeProgressError{
				err:            fmt.Errorf("mark quota committed: %w", err),
				quotaReserved:  true,
				quotaCommitted: true,
			}
		}
		saga.Status = SagaStatusQuotaCommitted
	}

	if details.ID == 0 {
		loadedDetails, err := o.subscriptions.GetSubscriptionDetailsForSaga(ctx, saga.SubscriptionID)
		if err != nil {
			return err
		}
		details = loadedDetails
	}

	return o.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := o.mailQueue.QueueSubscriptionConfirmation(ctx, details.Email, details.RepositoryFullName, details.ConfirmationToken, details.CancellationToken); err != nil {
			return err
		}

		return o.sagas.MarkCompleted(ctx, saga.ID)
	})
}

func (o *SagaOrchestrator) handleQuotaRejected(ctx context.Context, saga Saga, reason string) error {
	if reason == "" {
		reason = "quota rejected"
	}

	if err := o.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := o.subscriptions.DeleteByID(ctx, saga.SubscriptionID); err != nil {
			return err
		}

		return o.sagas.MarkRejected(ctx, saga, reason)
	}); err != nil {
		return err
	}

	slog.InfoContext(ctx, "subscription saga rejected by quota service", "saga_id", saga.ID.String(), "subscription_id", saga.SubscriptionID, "reason", reason)
	return fmt.Errorf("%w: %s", ErrQuotaRejected, reason)
}

func (o *SagaOrchestrator) processUnsubscribe(ctx context.Context, saga Saga) error {
	if err := o.quota.ReleaseSubscriptionSlot(ctx, saga.ID, saga.SubscriptionID); err != nil {
		return fmt.Errorf("release quota slot: %w", err)
	}

	return o.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := o.subscriptions.DeleteByID(ctx, saga.SubscriptionID); err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}

		return o.sagas.MarkCompleted(ctx, saga.ID)
	})
}

func (o *SagaOrchestrator) processCompensation(ctx context.Context, saga Saga) error {
	if saga.Operation != SagaOperationSubscribe {
		err := fmt.Errorf("unsupported compensation operation: %s", saga.Operation)
		if markErr := o.sagas.MarkCompensationFailed(ctx, saga, err, nextSagaRetryAt(saga.Attempts+1), defaultSagaMaxAttempts); markErr != nil {
			return fmt.Errorf("%w; mark compensation failed: %v", err, markErr)
		}

		return err
	}

	if err := o.compensateSubscribe(ctx, saga); err != nil {
		if markErr := o.sagas.MarkCompensationFailed(ctx, saga, err, nextSagaRetryAt(saga.Attempts+1), defaultSagaMaxAttempts); markErr != nil {
			return fmt.Errorf("%w; mark compensation failed: %v", err, markErr)
		}

		return err
	}

	return nil
}

func (o *SagaOrchestrator) compensateSubscribe(ctx context.Context, saga Saga) error {
	if err := o.quota.ReleaseSubscriptionSlot(ctx, saga.ID, saga.SubscriptionID); err != nil {
		return fmt.Errorf("release quota slot: %w", err)
	}

	return o.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := o.subscriptions.DeleteByID(ctx, saga.SubscriptionID); err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}

		return o.sagas.MarkCompensated(ctx, saga.ID)
	})
}

func (o *SagaOrchestrator) shouldCompensateSubscribe(saga Saga) bool {
	return saga.Operation == SagaOperationSubscribe && saga.Status == SagaStatusQuotaCommitted
}

func (o *SagaOrchestrator) shouldTerminallyFailSubscribeBeforeQuotaCommit(saga Saga) bool {
	return saga.Operation == SagaOperationSubscribe && saga.Status != SagaStatusQuotaCommitted && saga.Status != SagaStatusCompensating
}

func (o *SagaOrchestrator) markSubscribeDead(ctx context.Context, saga Saga, cause error) error {
	return o.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := o.subscriptions.DeleteByID(ctx, saga.SubscriptionID); err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}

		return o.sagas.MarkDead(ctx, saga, cause)
	})
}

func (o *SagaOrchestrator) releaseReservedQuotaBestEffort(ctx context.Context, saga Saga) {
	if err := o.quota.ReleaseSubscriptionSlot(ctx, saga.ID, saga.SubscriptionID); err != nil {
		slog.WarnContext(
			ctx,
			"subscription saga reserved quota release failed",
			"saga_id", saga.ID.String(),
			"subscription_id", saga.SubscriptionID,
			"error", err,
		)
	}
}

type SagaRetryWorker struct {
	orchestrator *SagaOrchestrator
	emptyDelay   time.Duration
}

func NewSagaRetryWorker(orchestrator *SagaOrchestrator) *SagaRetryWorker {
	return &SagaRetryWorker{
		orchestrator: orchestrator,
		emptyDelay:   time.Second,
	}
}

func (w *SagaRetryWorker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		err := w.orchestrator.ProcessNext(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				if !sleepContext(ctx, w.emptyDelay) {
					return
				}
				continue
			}

			slog.WarnContext(ctx, "subscription saga retry failed", "error", err)
		}
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextSagaRetryAt(attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}

	delay := defaultSagaRetryBaseDelay
	for i := 1; i < attempts && i < 6; i++ {
		delay *= 2
	}

	return time.Now().Add(delay)
}
