package main

import (
	"context"
	"errors"
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
	defer func() {
		if err := pg.Close(); err != nil {
			log.Printf("close postgres: %v", err)
		}
	}()

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

	if err := validateMailConfig(cfg.Mail); err != nil {
		log.Fatalf("invalid mail config: %v", err)
	}

	sender := newMailSender(cfg.Mail)
	mailService := mail.NewService(templateRenderer, outboxStore, cfg.Mail.ApiBaseUrl)
	outboxDispatcher := mail.NewOutboxDispatcher(outboxStore, sender)

	subscriptionService := service.NewSubscriptionService(
		transactionManager,
		userStore,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		mailService,
	)
	releaseMonitor := service.NewReleaseMonitor(
		transactionManager,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		mailService,
	)

	router := httpapi.NewRouter(httpapi.NewSubscriptionHandler(subscriptionService))

	go outboxDispatcher.Run(appCtx)
	go releaseMonitor.Run(appCtx)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("run http server: %v", err)
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
