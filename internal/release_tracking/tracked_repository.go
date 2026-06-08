package releasetracking

import "time"

type TrackedRepository struct {
	ID          int64
	Owner       string
	Name        string
	LastSeenTag *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
