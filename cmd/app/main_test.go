//go:build unit

package main

import (
	"testing"

	"github-release-notifier/internal/platform/config"

	"github.com/stretchr/testify/require"
)

func TestValidateConfig_RequiresFields(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(cfg *config.Config)
		expectedErr string
	}{
		{
			name: "server port",
			mutate: func(cfg *config.Config) {
				cfg.Server.Port = ""
			},
			expectedErr: "server.port is required",
		},
		{
			name: "database url",
			mutate: func(cfg *config.Config) {
				cfg.Database.URL = ""
			},
			expectedErr: "database.url is required",
		},
		{
			name: "rabbitmq url",
			mutate: func(cfg *config.Config) {
				cfg.RabbitMQ.URL = ""
			},
			expectedErr: "rabbitmq.url is required",
		},
		{
			name: "rabbitmq exchange",
			mutate: func(cfg *config.Config) {
				cfg.RabbitMQ.NotificationExchange = ""
			},
			expectedErr: "rabbitmq.notification_exchange is required",
		},
		{
			name: "rabbitmq queue",
			mutate: func(cfg *config.Config) {
				cfg.RabbitMQ.NotificationQueue = ""
			},
			expectedErr: "rabbitmq.notification_queue is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAppConfig()
			tt.mutate(&cfg)

			err := validateConfig(cfg)

			require.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestValidateConfig_AcceptsValidConfig(t *testing.T) {
	err := validateConfig(validAppConfig())

	require.NoError(t, err)
}

func validAppConfig() config.Config {
	cfg := config.Default()
	cfg.Database.URL = "postgres://user:pass@localhost:5432/db?sslmode=disable"
	cfg.RabbitMQ.URL = "amqp://guest:guest@localhost:5672/"
	return cfg
}
