package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github-release-notifier/internal/platform/config"
	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/logging"
	grpcserver "github-release-notifier/internal/quota/platform/grpc"
	"github-release-notifier/internal/quota/quotas"
	quotasrepo "github-release-notifier/internal/quota/quotas/repository"

	"google.golang.org/grpc"
)

func main() {
	bootstrapLogger, _ := logging.New("", "")
	slog.SetDefault(bootstrapLogger)

	cfg, err := config.LoadQuota()
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

	if err := validateQuotaConfig(cfg); err != nil {
		logger.Error("validate quota config", "error", err)
		os.Exit(1)
	}

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	service := buildService(pg, cfg)
	grpcServer := grpc.NewServer()
	grpcserver.Register(grpcServer, service)

	logger.Info(
		"quota service starting",
		"grpc_port", cfg.Quota.GRPCPort,
		"default_subscription_limit", cfg.Quota.DefaultSubscriptionLimit,
	)

	go func() {
		<-appCtx.Done()
		grpcServer.GracefulStop()
	}()

	listener, err := net.Listen("tcp", ":"+cfg.Quota.GRPCPort)
	if err != nil {
		logger.Error("listen grpc", "error", err)
		os.Exit(1)
	}

	if err := grpcServer.Serve(listener); err != nil {
		logger.Error("serve grpc", "error", err)
		os.Exit(1)
	}

	logger.Info("quota service stopped")
}

func openDatabase(ctx context.Context, datasourceURL string) (*sql.DB, error) {
	pg, err := appdb.OpenPostgres(ctx, datasourceURL)
	if err != nil {
		return nil, err
	}

	if err := appdb.RunMigrations(ctx, pg, "migrations/quota"); err != nil {
		_ = pg.Close()
		return nil, err
	}

	return pg, nil
}

func buildService(pg *sql.DB, cfg config.Config) *quotas.Service {
	reservationStore := quotasrepo.NewReservationStore(pg)
	return quotas.NewService(reservationStore, cfg.Quota.DefaultSubscriptionLimit)
}

func validateQuotaConfig(cfg config.Config) error {
	switch {
	case cfg.Database.URL == "":
		return errors.New("database.url is required")
	case cfg.Quota.GRPCPort == "":
		return errors.New("quota.grpc_port is required")
	case cfg.Quota.DefaultSubscriptionLimit <= 0:
		return errors.New("quota.default_subscription_limit must be positive")
	default:
		return nil
	}
}
