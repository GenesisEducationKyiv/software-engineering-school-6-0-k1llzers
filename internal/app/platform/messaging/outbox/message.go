package outbox

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID                  int64
	MessageID           uuid.UUID
	MessageType         string
	PayloadJSON         json.RawMessage
	ProcessingStartedAt *time.Time
	PublishedAt         *time.Time
	CreatedAt           time.Time
}
