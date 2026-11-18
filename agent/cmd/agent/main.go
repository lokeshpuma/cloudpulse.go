package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/collector"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/server"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/servicecheck"
)

// Version is set at build time via -ldflags.
var Version = "1.0.0"

func main() {
	configPath := flag.String("config", "", "Path to YAML configuration file")
	showVersion := flag.Bool("version", false, "Print agent version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("cloudpulse-agent %s\n", Version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := buildLogger(cfg.Logging)
	logger.Info("starting cloudpulse-agent",
		"version", Version,
		"agent_id", cfg.Agent.ID,
		"agent_name", cfg.Agent.Name,
		"environment", cfg.Agent.Environment,
		"listen_address", cfg.Server.ListenAddress,
	)

	coll := collector.NewManager(cfg, Version, logger, nil)
	svcRunner := servicecheck.NewRunner(cfg, coll.Registry(), logger)
	httpSrv := server.New(cfg, coll, svcRunner, Version, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		coll.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		svcRunner.Run(ctx)
	}()

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining agent")
	case err := <-errCh:
		if err != nil {
			logger.Error("agent HTTP server failed", "error", err)
			stop()
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}
	wg.Wait()
	logger.Info("cloudpulse-agent stopped cleanly")
}

func buildLogger(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(cfg.Format, "text") {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
