package quota

import (
	"context"

	"github-release-notifier/internal/app/subscriptions"
	quotapb "github-release-notifier/pkg/contracts/quota"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type Client struct {
	client quotapb.QuotaServiceClient
}

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{client: quotapb.NewQuotaServiceClient(conn)}
}

func (c *Client) ReserveSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64, email string) (subscriptions.QuotaReserveResult, error) {
	response, err := c.client.ReserveSubscriptionSlot(ctx, &quotapb.ReserveSubscriptionSlotRequest{
		SagaId:         sagaID.String(),
		SubscriptionId: subscriptionID,
		Email:          email,
	})
	if err != nil {
		return subscriptions.QuotaReserveResult{}, err
	}

	return subscriptions.QuotaReserveResult{
		Reserved:        response.GetReserved(),
		RejectionReason: response.GetRejectionReason(),
	}, nil
}

func (c *Client) CommitSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64) error {
	_, err := c.client.CommitSubscriptionSlot(ctx, &quotapb.CommitSubscriptionSlotRequest{
		SagaId:         sagaID.String(),
		SubscriptionId: subscriptionID,
	})
	return err
}

func (c *Client) ReleaseSubscriptionSlot(ctx context.Context, sagaID uuid.UUID, subscriptionID int64) error {
	_, err := c.client.ReleaseSubscriptionSlot(ctx, &quotapb.ReleaseSubscriptionSlotRequest{
		SagaId:         sagaID.String(),
		SubscriptionId: subscriptionID,
	})
	return err
}
