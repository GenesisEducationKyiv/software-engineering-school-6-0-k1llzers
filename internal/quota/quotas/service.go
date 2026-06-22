package quotas

import (
	"context"

	"github.com/google/uuid"
)

type reservationStore interface {
	ReserveSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64, email string, defaultLimit int) (ReserveResult, error)
	CommitSlot(ctx context.Context, subscriptionID int64) error
	ReleaseSlot(ctx context.Context, subscriptionID int64) error
}

type Service struct {
	reservations             reservationStore
	defaultSubscriptionLimit int
}

func NewService(reservations reservationStore, defaultSubscriptionLimit int) *Service {
	return &Service{
		reservations:             reservations,
		defaultSubscriptionLimit: defaultSubscriptionLimit,
	}
}

func (s *Service) ReserveSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64, email string) (ReserveResult, error) {
	return s.reservations.ReserveSlot(ctx, sagaID, subscriptionID, email, s.defaultSubscriptionLimit)
}

func (s *Service) CommitSubscriptionSlot(ctx context.Context, subscriptionID int64) error {
	return s.reservations.CommitSlot(ctx, subscriptionID)
}

func (s *Service) ReleaseSubscriptionSlot(ctx context.Context, subscriptionID int64) error {
	return s.reservations.ReleaseSlot(ctx, subscriptionID)
}
