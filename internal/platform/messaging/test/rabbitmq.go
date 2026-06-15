//go:build integration

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedRabbitMQOnce      sync.Once
	sharedRabbitMQContainer testcontainers.Container
	sharedRabbitMQURL       string
	sharedRabbitMQErr       error
)

func SetupRabbitMQ(t *testing.T) string {
	t.Helper()

	sharedRabbitMQOnce.Do(func() {
		ctx := context.Background()
		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "rabbitmq:4.1-management",
				ExposedPorts: []string{"5672/tcp"},
				WaitingFor:   wait.ForListeningPort("5672/tcp"),
			},
			Started: true,
		})
		if err != nil {
			sharedRabbitMQErr = err
			return
		}

		host, err := container.Host(ctx)
		if err != nil {
			sharedRabbitMQErr = err
			return
		}

		port, err := container.MappedPort(ctx, "5672/tcp")
		if err != nil {
			sharedRabbitMQErr = err
			return
		}

		sharedRabbitMQContainer = container
		sharedRabbitMQURL = fmt.Sprintf("amqp://guest:guest@%s:%s/", host, port.Port())
		sharedRabbitMQErr = waitForRabbitMQ(ctx, sharedRabbitMQURL, 30*time.Second)
	})

	require.NoError(t, sharedRabbitMQErr)
	return sharedRabbitMQURL
}

func OpenRabbitMQChannel(t *testing.T, rabbitMQURL string) (*amqp.Connection, *amqp.Channel) {
	t.Helper()

	conn, err := amqp.Dial(rabbitMQURL)
	require.NoError(t, err)

	channel, err := conn.Channel()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, channel.Close())
		require.NoError(t, conn.Close())
	})

	return conn, channel
}

func NewTestBrokerName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.NewString())
}

func waitForRabbitMQ(ctx context.Context, rabbitMQURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := amqp.Dial(rabbitMQURL)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for rabbitmq")
}
