package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"

	"github-release-notifier/internal/app/notifications"
	"github-release-notifier/internal/app/platform/github"
	"github-release-notifier/internal/app/platform/http/api"
	integrationoutbox "github-release-notifier/internal/app/platform/messaging/outbox"
	"github-release-notifier/internal/app/platform/messaging/rabbitmq"
	"github-release-notifier/internal/app/platform/metrics"
	"github-release-notifier/internal/app/release_tracking"
	releasetrackingrepo "github-release-notifier/internal/app/release_tracking/repository"
	"github-release-notifier/internal/app/subscriptions"
	subscriptionsrepo "github-release-notifier/internal/app/subscriptions/repository"
	"github-release-notifier/internal/platform/config"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/logging"

	"github.com/gin-gonic/gin"
)

func main() {
	bootstrapLogger, _ := logging.New(config.Default().Logging.Level, config.Default().Logging.Format)
	slog.SetDefault(bootstrapLogger)

	cfg, err := config.LoadApp()
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

	appCtx := context.Background()
	appMetrics, err := metrics.New()
	if err != nil {
		logger.Error("initialize metrics", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := appMetrics.Shutdown(appCtx); err != nil {
			logger.Warn("shutdown metrics failed", "error", err)
		}
	}()

	logger.Info("application starting", "port", cfg.Server.Port)

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

	app, err := buildApplication(pg, cfg, appMetrics)
	if err != nil {
		logger.Error("build application", "error", err)
		os.Exit(1)
	}

	workers := []BackgroundWorker{app.releaseMonitor}
	if app.integrationOutboxPublisher != nil {
		workers = append(workers, app.integrationOutboxPublisher)
	}
	startBackgroundWorkers(appCtx, workers...)

	if err := app.router.Run(":" + cfg.Server.Port); err != nil {
		logger.Error("run http server", "error", err)
		os.Exit(1)
	}
}

type application struct {
	router                     *gin.Engine
	integrationOutboxPublisher *integrationoutbox.PublisherWorker
	releaseMonitor             *releasetracking.ReleaseMonitor
}

func openDatabase(ctx context.Context, datasourceURL string) (*sql.DB, error) {
	pg, err := appdb.OpenPostgres(ctx, datasourceURL)
	if err != nil {
		return nil, err
	}

	if err := appdb.RunMigrations(ctx, pg, "migrations/app"); err != nil {
		_ = pg.Close()
		return nil, err
	}

	return pg, nil
}

func buildApplication(pg *sql.DB, cfg config.Config, appMetrics *metrics.Metrics) (*application, error) {
	transactionManager := appdb.NewTransactionManager(pg)
	userStore := subscriptionsrepo.NewUserStore(pg)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(pg)
	subscriptionStore := subscriptionsrepo.NewSubscriptionStore(pg)
	confirmedSubscriptionStore := releasetrackingrepo.NewConfirmedSubscriptionStore(pg)
	integrationOutboxStore := integrationoutbox.NewStore(pg)
	githubClient := github.NewClient(nil, cfg.GitHub.Token)
	integrationPublisher := newIntegrationOutboxPublisher(cfg.RabbitMQ, integrationOutboxStore)
	notificationService := notifications.NewService(integrationOutboxStore)
	subscriptionService := subscriptions.NewService(
		transactionManager,
		userStore,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		notificationService,
	)

	return &application{
		router:                     httpapi.NewRouter(httpapi.NewSubscriptionHandler(subscriptionService), appMetrics),
		integrationOutboxPublisher: integrationPublisher,
		releaseMonitor: releasetracking.NewReleaseMonitor(
			transactionManager,
			trackedRepositoryStore,
			confirmedSubscriptionStore,
			githubClient,
			notificationService,
			appMetrics,
		),
	}, nil
}

type BackgroundWorker interface {
	Run(context.Context)
}

func startBackgroundWorkers(ctx context.Context, workers ...BackgroundWorker) {
	for _, worker := range workers {
		go worker.Run(ctx)
	}
}

func newIntegrationOutboxPublisher(cfg config.RabbitMQConfig, store *integrationoutbox.Store) *integrationoutbox.PublisherWorker {
	if cfg.URL == "" {
		return nil
	}

	return integrationoutbox.NewPublisherWorker(
		store,
		rabbitmq.NewPublisher(cfg.URL, cfg.NotificationExchange),
	)
}
