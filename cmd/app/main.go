package main

import (
	"context"
	"log"

	"github-release-notifier/internal/config"
	"github-release-notifier/internal/db"
	"github-release-notifier/internal/github"
	"github-release-notifier/internal/httpapi"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/service"
	"github-release-notifier/internal/storage"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pg, err := db.OpenPostgres(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer pg.Close()

	if err := db.RunMigrations(context.Background(), pg, "migrations"); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	transactionManager := db.NewTransactionManager(pg)
	appCtx := context.Background()
	userStore := storage.NewUserStore(pg)
	trackedRepositoryStore := storage.NewTrackedRepositoryStore(pg)
	subscriptionStore := storage.NewSubscriptionStore(pg)
	outboxStore := storage.NewOutboxStore(pg)
	githubClient := github.NewClient(nil, cfg.GitHub.Token)
	templateRenderer, err := mail.NewTemplateRenderer()
	if err != nil {
		log.Fatalf("create mail template renderer: %v", err)
	}

	sender := mail.Sender(mail.NewNoopSender())
	if cfg.Mail.Host != "" && cfg.Mail.From != "" && cfg.Mail.ApiBaseUrl != "" {
		sender = mail.NewSMTPSender(mail.SMTPConfig{
			Host:     cfg.Mail.Host,
			Port:     cfg.Mail.Port,
			Username: cfg.Mail.Username,
			Password: cfg.Mail.Password,
			From:     cfg.Mail.From,
		})
	}

	mailService := mail.NewService(templateRenderer, outboxStore, cfg.Mail.ApiBaseUrl)
	mailDispatcher := mail.NewDispatcher(outboxStore, sender)

	subscriptionService := service.NewSubscriptionServiceFromStorage(
		transactionManager,
		userStore,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		mailService,
	)
	releaseChecker := service.NewReleaseCheckerServiceFromStorage(
		transactionManager,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		mailService,
	)

	router := httpapi.NewRouter(httpapi.NewSubscriptionHandler(subscriptionService))

	go mailDispatcher.Run(appCtx)
	go releaseChecker.Run(appCtx)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("run http server: %v", err)
	}
}
