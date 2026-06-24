package grpcserver

import (
	"context"
	"errors"

	"github-release-notifier/internal/quota/quotas"
	quotapb "github-release-notifier/pkg/contracts/quota"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	quotapb.UnimplementedQuotaServiceServer

	quotas *quotas.Service
}

func Register(server *grpc.Server, quotas *quotas.Service) {
	quotapb.RegisterQuotaServiceServer(server, &Server{quotas: quotas})
}

func (s *Server) ReserveSubscriptionSlot(ctx context.Context, req *quotapb.ReserveSubscriptionSlotRequest) (*quotapb.ReserveSubscriptionSlotResponse, error) {
	sagaID, err := uuid.Parse(req.GetSagaId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid saga_id")
	}

	if err := validateSubscriptionID(req.GetSubscriptionId()); err != nil {
		return nil, err
	}

	if req.GetEmail() == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	result, err := s.quotas.ReserveSubscriptionSlot(ctx, sagaID, req.GetSubscriptionId(), req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.Internal, "reserve subscription slot failed")
	}

	return &quotapb.ReserveSubscriptionSlotResponse{
		Reserved:        result.Reserved,
		RejectionReason: result.RejectionReason,
	}, nil
}

func (s *Server) CommitSubscriptionSlot(ctx context.Context, req *quotapb.CommitSubscriptionSlotRequest) (*quotapb.CommitSubscriptionSlotResponse, error) {
	if err := validateSubscriptionID(req.GetSubscriptionId()); err != nil {
		return nil, err
	}

	if err := s.quotas.CommitSubscriptionSlot(ctx, req.GetSubscriptionId()); err != nil {
		return nil, mapQuotaError(err, "commit subscription slot failed")
	}

	return &quotapb.CommitSubscriptionSlotResponse{}, nil
}

func (s *Server) ReleaseSubscriptionSlot(ctx context.Context, req *quotapb.ReleaseSubscriptionSlotRequest) (*quotapb.ReleaseSubscriptionSlotResponse, error) {
	if err := validateSubscriptionID(req.GetSubscriptionId()); err != nil {
		return nil, err
	}

	if err := s.quotas.ReleaseSubscriptionSlot(ctx, req.GetSubscriptionId()); err != nil {
		return nil, status.Error(codes.Internal, "release subscription slot failed")
	}

	return &quotapb.ReleaseSubscriptionSlotResponse{}, nil
}

func validateSubscriptionID(subscriptionID int64) error {
	if subscriptionID <= 0 {
		return status.Error(codes.InvalidArgument, "subscription_id must be positive")
	}

	return nil
}

func mapQuotaError(err error, fallbackMessage string) error {
	switch {
	case errors.Is(err, quotas.ErrReservationNotFound):
		return status.Error(codes.NotFound, "quota reservation not found")
	case errors.Is(err, quotas.ErrReservationCannotBeCommitted):
		return status.Error(codes.FailedPrecondition, "quota reservation cannot be committed")
	default:
		return status.Error(codes.Internal, fallbackMessage)
	}
}
