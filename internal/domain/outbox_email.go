package domain

import "time"

type OutboxEmail struct {
	ID                  int64
	RecipientEmail      string
	Subject             string
	HTMLBody            string
	TextBody            string
	Attempts            int
	ProcessingStartedAt *time.Time
	SentAt              *time.Time
	LastError           *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
