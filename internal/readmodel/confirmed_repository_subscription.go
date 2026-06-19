package readmodel

import "github.com/google/uuid"

type ConfirmedRepositorySubscription struct {
	TrackedRepositoryID int64
	Owner               string
	Name                string
	LastSeenTag         string
	Email               string
	CancellationToken   uuid.UUID
}
