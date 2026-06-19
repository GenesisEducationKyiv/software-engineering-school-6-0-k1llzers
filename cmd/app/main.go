package main

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"github-release-notifier/internal/config"
	"github-release-notifier/internal/db"
	"github-release-notifier/internal/github"
	"github-release-notifier/internal/httpapi"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/service"
	"github-release-notifier/internal/storage"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	appCtx := context.Background()
	pg, err := openDatabase(appCtx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() {
		if err := pg.Close(); err != nil {
			log.Printf("close postgres: %v", err)
		}
	}()

	app, err := buildApplication(pg, cfg)
	if err != nil {
		log.Fatalf("build application: %v", err)
	}

	startBackgroundWorkers(appCtx, app.outboxDispatcher, app.releaseMonitor)

	if err := app.router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("run http server: %v", err)
	}
}

type application struct {
	router           *gin.Engine
	outboxDispatcher *mail.OutboxDispatcher
	releaseMonitor   *service.ReleaseMonitor
}

func openDatabase(ctx context.Context, datasourceURL string) (*sql.DB, error) {
	pg, err := db.OpenPostgres(ctx, datasourceURL)
	if err != nil {
		return nil, err
	}

	if err := db.RunMigrations(ctx, pg, "migrations"); err != nil {
		_ = pg.Close()
		return nil, err
	}

	return pg, nil
}

func buildApplication(pg *sql.DB, cfg config.Config) (*application, error) {
	if err := validateMailConfig(cfg.Mail); err != nil {
		return nil, err
	}

	transactionManager := db.NewTransactionManager(pg)
	userStore := storage.NewUserStore(pg)
	trackedRepositoryStore := storage.NewTrackedRepositoryStore(pg)
	subscriptionStore := storage.NewSubscriptionStore(pg)
	outboxStore := storage.NewOutboxStore(pg)
	githubClient := github.NewClient(nil, cfg.GitHub.Token)
	templateRenderer, err := mail.NewTemplateRenderer()
	if err != nil {
		return nil, err
	}

	sender := newMailSender(cfg.Mail)
	mailService := mail.NewService(templateRenderer, outboxStore, cfg.Mail.ApiBaseUrl)
	subscriptionService := service.NewSubscriptionService(
		transactionManager,
		userStore,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		mailService,
	)

	return &application{
		router:           httpapi.NewRouter(httpapi.NewSubscriptionHandler(subscriptionService)),
		outboxDispatcher: mail.NewOutboxDispatcher(outboxStore, sender),
		releaseMonitor: service.NewReleaseMonitor(
			transactionManager,
			trackedRepositoryStore,
			subscriptionStore,
			githubClient,
			mailService,
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

func newMailSender(cfg config.MailConfig) mail.Sender {
	return mail.NewSMTPSender(mail.SMTPConfig{
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
