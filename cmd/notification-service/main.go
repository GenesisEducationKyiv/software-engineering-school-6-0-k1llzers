package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"

	"github-release-notifier/internal/notification/notifications"
	notificationsrepo "github-release-notifier/internal/notification/notifications/repository"
	"github-release-notifier/internal/notification/platform/messaging/rabbitmq"
	"github-release-notifier/internal/platform/config"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/logging"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"
)

func main() {
	bootstrapLogger, _ := logging.New(config.Default().Logging.Level, config.Default().Logging.Format)
	slog.SetDefault(bootstrapLogger)

	cfg, err := config.LoadNotification()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	logger, err := logging.New(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		slog.Error("initialize logger", "error", err, "level", cfg.Logging.Level, "format", cfg.Logging.Format)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	if err := validateRabbitMQConfig(cfg.RabbitMQ); err != nil {
		logger.Error("validate rabbitmq config", "error", err)
		os.Exit(1)
	}

	appCtx := context.Background()
	pg, err := openDatabase(appCtx, cfg.Database.URL)
	if err != nil {
		logger.Error("initialize database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := pg.Close(); err != nil {
			logger.Warn("close postgres failed", "error", err)
		}
	}()

	consumer := buildConsumer(pg, cfg)
	logger.Info("notification service starting", "queue", cfg.RabbitMQ.NotificationQueue)
	consumer.Run(appCtx)
}

func openDatabase(ctx context.Context, datasourceURL string) (*sql.DB, error) {
	pg, err := appdb.OpenPostgres(ctx, datasourceURL)
	if err != nil {
		return nil, err
	}

	if err := appdb.RunMigrations(ctx, pg, "migrations/notification"); err != nil {
		_ = pg.Close()
		return nil, err
	}

	return pg, nil
}

func buildConsumer(pg *sql.DB, cfg config.Config) *rabbitmq.Consumer {
	messageInboxStore := notificationsrepo.NewMessageInboxStore(pg)
	handler := notifications.NewMessageInboxHandler(messageInboxStore, nil)

	return rabbitmq.NewConsumer(
		cfg.RabbitMQ.URL,
		cfg.RabbitMQ.NotificationExchange,
		cfg.RabbitMQ.NotificationQueue,
		[]string{
			string(notificationcontracts.TypeSubscriptionConfirmationRequested),
			string(notificationcontracts.TypeReleaseNotificationRequested),
		},
		handler,
	)
}

func validateRabbitMQConfig(cfg config.RabbitMQConfig) error {
	switch {
	case cfg.URL == "":
		return errors.New("rabbitmq.url is required")
	case cfg.NotificationExchange == "":
		return errors.New("rabbitmq.notification_exchange is required")
	case cfg.NotificationQueue == "":
		return errors.New("rabbitmq.notification_queue is required")
	default:
		return nil
	}
}
