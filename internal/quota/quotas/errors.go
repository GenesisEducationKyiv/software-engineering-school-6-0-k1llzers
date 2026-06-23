package quotas

import "errors"

var ErrReservationNotFound = errors.New("quota reservation not found")

var ErrReservationCannotBeCommitted = errors.New("quota reservation cannot be committed")

var ErrReservationSagaMismatch = errors.New("quota reservation belongs to another saga")
