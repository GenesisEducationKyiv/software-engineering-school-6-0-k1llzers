package outbox

import "time"

type Email struct {
	ID                  int64
	RecipientEmail      string
	Subject             string
	HTMLBody            string
	Attempts            int
	ProcessingStartedAt *time.Time
	SentAt              *time.Time
	LastError           *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
