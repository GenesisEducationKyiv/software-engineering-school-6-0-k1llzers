package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github-release-notifier/internal/notification/notifications"
	notificationsrepo "github-release-notifier/internal/notification/notifications/repository"
	"github-release-notifier/internal/notification/platform/mail/smtp"
	"github-release-notifier/internal/notification/platform/messaging/rabbitmq"
	notificationmetrics "github-release-notifier/internal/notification/platform/metrics"
	"github-release-notifier/internal/platform/config"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/logging"
	notificationcontracts "github-release-notifier/pkg/contracts/notifications"
)

func main() {
	bootstrapLogger, _ := logging.New("", "")
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

	if err := validateConfig(cfg); err != nil {
		logger.Error("validate notification service config", "error", err)
		os.Exit(1)
	}

	appCtx := context.Background()
	metricSet, err := notificationmetrics.New()
	if err != nil {
		logger.Error("initialize notification metrics", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := metricSet.Shutdown(appCtx); err != nil {
			logger.Warn("shutdown notification metrics failed", "error", err)
		}
	}()

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

	consumer, err := buildConsumer(pg, cfg, metricSet)
	if err != nil {
		logger.Error("build notification consumer", "error", err)
		os.Exit(1)
	}
	go consumer.Run(appCtx)

	logger.Info(
		"notification service starting",
		"queue", cfg.RabbitMQ.NotificationQueue,
		"metrics_port", cfg.Server.Port,
	)

	if err := http.ListenAndServe(":"+cfg.Server.Port, metricSet.Handler()); err != nil {
		logger.Error("run notification metrics server", "error", err)
		os.Exit(1)
	}
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

func buildConsumer(pg *sql.DB, cfg config.Config, metricSet *notificationmetrics.Metrics) (*rabbitmq.Consumer, error) {
	messageInboxStore := notificationsrepo.NewMessageInboxStore(pg)
	renderer, err := notifications.NewTemplateRenderer()
	if err != nil {
		return nil, err
	}

	sender := smtp.NewSender(smtp.Config{
		Host:     cfg.Mail.Host,
		Port:     cfg.Mail.Port,
		Username: cfg.Mail.Username,
		Password: cfg.Mail.Password,
		From:     cfg.Mail.From,
	})
	deliveryService := notifications.NewDeliveryService(renderer, sender, cfg.Mail.ApiBaseUrl, metricSet)
	handler := notifications.NewMessageInboxHandler(messageInboxStore, deliveryService)

	return rabbitmq.NewConsumer(
		cfg.RabbitMQ.URL,
		cfg.RabbitMQ.NotificationExchange,
		cfg.RabbitMQ.NotificationQueue,
		[]string{
			string(notificationcontracts.TypeSubscriptionConfirmationRequested),
			string(notificationcontracts.TypeReleaseNotificationRequested),
		},
		handler,
	), nil
}

func validateConfig(cfg config.Config) error {
	switch {
	case cfg.RabbitMQ.URL == "":
		return errors.New("rabbitmq.url is required")
	case cfg.RabbitMQ.NotificationExchange == "":
		return errors.New("rabbitmq.notification_exchange is required")
	case cfg.RabbitMQ.NotificationQueue == "":
		return errors.New("rabbitmq.notification_queue is required")
	case cfg.Server.Port == "":
		return errors.New("server.port is required")
	case cfg.Mail.Host == "":
		return errors.New("mail.host is required")
	case cfg.Mail.From == "":
		return errors.New("mail.from is required")
	case cfg.Mail.ApiBaseUrl == "":
		return errors.New("mail.api_base_url is required")
	default:
		return nil
	}
}
