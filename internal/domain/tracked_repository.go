package domain

import "time"

type TrackedRepository struct {
	ID          int64
	FullName    string
	LastSeenTag *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
