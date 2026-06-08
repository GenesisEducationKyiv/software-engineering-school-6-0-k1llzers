package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"

	"github-release-notifier/internal/notifications"
	notificationsrepo "github-release-notifier/internal/notifications/repository"
	"github-release-notifier/internal/platform/config"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/github"
	"github-release-notifier/internal/platform/http/api"
	"github-release-notifier/internal/platform/logging"
	"github-release-notifier/internal/platform/mail/smtp"
	"github-release-notifier/internal/platform/metrics"
	"github-release-notifier/internal/release_tracking"
	releasetrackingrepo "github-release-notifier/internal/release_tracking/repository"
	"github-release-notifier/internal/subscriptions"
	subscriptionsrepo "github-release-notifier/internal/subscriptions/repository"

	"github.com/gin-gonic/gin"
)

func main() {
	bootstrapLogger, _ := logging.New(config.Default().Logging.Level, config.Default().Logging.Format)
	slog.SetDefault(bootstrapLogger)

	cfg, err := config.Load("")
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

	startBackgroundWorkers(appCtx, app.outboxDispatcher, app.releaseMonitor)

	if err := app.router.Run(":" + cfg.Server.Port); err != nil {
		logger.Error("run http server", "error", err)
		os.Exit(1)
	}
}

type application struct {
	router           *gin.Engine
	outboxDispatcher *notifications.OutboxDispatcher
	releaseMonitor   *releasetracking.ReleaseMonitor
}

func openDatabase(ctx context.Context, datasourceURL string) (*sql.DB, error) {
	pg, err := appdb.OpenPostgres(ctx, datasourceURL)
	if err != nil {
		return nil, err
	}

	if err := appdb.RunMigrations(ctx, pg, "migrations"); err != nil {
		_ = pg.Close()
		return nil, err
	}

	return pg, nil
}

func buildApplication(pg *sql.DB, cfg config.Config, appMetrics *metrics.Metrics) (*application, error) {
	if err := validateMailConfig(cfg.Mail); err != nil {
		return nil, err
	}

	transactionManager := appdb.NewTransactionManager(pg)
	userStore := subscriptionsrepo.NewUserStore(pg)
	trackedRepositoryStore := releasetrackingrepo.NewTrackedRepositoryStore(pg)
	subscriptionStore := subscriptionsrepo.NewSubscriptionStore(pg)
	confirmedSubscriptionStore := releasetrackingrepo.NewConfirmedSubscriptionStore(pg)
	outboxStore := notificationsrepo.NewOutboxStore(pg)
	githubClient := github.NewClient(nil, cfg.GitHub.Token)
	templateRenderer, err := notifications.NewTemplateRenderer()
	if err != nil {
		return nil, err
	}

	sender := newMailSender(cfg.Mail)
	notificationService := notifications.NewService(templateRenderer, outboxStore, cfg.Mail.ApiBaseUrl)
	subscriptionService := subscriptions.NewService(
		transactionManager,
		userStore,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		notificationService,
	)

	return &application{
		router:           httpapi.NewRouter(httpapi.NewSubscriptionHandler(subscriptionService), appMetrics),
		outboxDispatcher: notifications.NewOutboxDispatcher(outboxStore, sender, appMetrics),
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

func newMailSender(cfg config.MailConfig) *smtp.Sender {
	return smtp.NewSender(smtp.Config{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     cfg.From,
	})
}

func validateMailConfig(cfg config.MailConfig) error {
	switch {
	case cfg.Host == "":
		return errors.New("mail.host is required")
	case cfg.From == "":
		return errors.New("mail.from is required")
	case cfg.ApiBaseUrl == "":
		return errors.New("mail.api_base_url is required")
	default:
		return nil
	}
}
