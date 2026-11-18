package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/alertmanager"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/audit"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/auth"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/config"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/database"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/discovery"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/grafana"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/handlers"
	prom "github.com/cloudpulse/cloudpulse-go/backend/internal/prometheus"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/services"
	"github.com/cloudpulse/cloudpulse-go/backend/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := buildLogger(cfg.LogLevel)
	logger.Info("starting cloudpulse backend api", "env", cfg.AppEnv, "port", cfg.AppPort, "version", cfg.Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Error("failed to connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := database.RunMigrations(ctx, pool, migrations.FS, logger); err != nil {
		logger.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}

	store := repository.NewPGStore(pool)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	promClient := prom.NewHTTPClient(cfg.PrometheusURL, 8*time.Second)
	amClient := alertmanager.NewHTTPClient(cfg.AlertmanagerURL, 5*time.Second)
	gfClient := grafana.NewClient(cfg.GrafanaURL, cfg.GrafanaPublicURL, 4*time.Second)
	disc := discovery.NewManager(cfg.PrometheusFileSDPath, cfg.PrometheusRulesDir, cfg.PrometheusReloadEnabled, promClient, logger)
	aud := audit.NewRecorder(store, logger)
	monSvc := services.NewMonitoringService(store, promClient, amClient, gfClient, disc, logger)

	// Synchronize Prometheus file_sd targets on startup so discovery matches PostgreSQL immediately
	if err := monSvc.SyncDiscovery(ctx); err != nil {
		logger.Warn("initial prometheus file_sd sync failed", "error", err)
	}

	api := handlers.NewAPI(cfg, store, tokens, monSvc, promClient, amClient, gfClient, aud, logger)
	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           api.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("backend api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			logger.Error("http server error", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	logger.Info("cloudpulse backend api stopped cleanly")
}

func buildLogger(levelStr string) *slog.Logger {
	var lvl slog.Level
	switch levelStr {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
