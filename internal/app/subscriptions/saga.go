package subscriptions

import (
	"time"

	"github.com/google/uuid"
)

type SagaOperation string

const (
	SagaOperationSubscribe   SagaOperation = "subscribe"
	SagaOperationUnsubscribe SagaOperation = "unsubscribe"
)

type SagaStatus string

const (
	SagaStatusPending        SagaStatus = "pending"
	SagaStatusRunning        SagaStatus = "running"
	SagaStatusQuotaCommitted SagaStatus = "quota_committed"
	SagaStatusCompleted      SagaStatus = "completed"
	SagaStatusFailed         SagaStatus = "failed"
	SagaStatusCompensating   SagaStatus = "compensating"
	SagaStatusCompensated    SagaStatus = "compensated"
	SagaStatusRejected       SagaStatus = "rejected"
	SagaStatusDead           SagaStatus = "dead"
)

type Saga struct {
	ID                  uuid.UUID
	SubscriptionID      int64
	Operation           SagaOperation
	Status              SagaStatus
	Attempts            int
	LastError           *string
	ProcessingStartedAt *time.Time
	NextRetryAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SagaSubscriptionDetails struct {
	Subscription
	Email              string
	RepositoryFullName string
}
