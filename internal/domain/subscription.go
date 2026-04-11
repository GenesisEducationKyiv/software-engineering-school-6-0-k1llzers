package domain

import (
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID                  int64
	UserID              int64
	TrackedRepositoryID int64
	Confirmed           bool
	ConfirmationToken   uuid.UUID
	CancellationToken   uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
