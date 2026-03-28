package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"prs/internal/api"
	"prs/internal/cleanup"
	"prs/internal/config"
	"prs/internal/recovery"
	"prs/internal/staticfiles"
	"prs/internal/store"
	"prs/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Configure structured logging. log/slog is stdlib since Go 1.21.
	// PRS_LOG_FORMAT=json → machine-readable (for prod/log aggregators).
	// PRS_LOG_LEVEL=debug → verbose output for local development.
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	handlerOpts := &slog.HandlerOptions{Level: logLevel}
	var logHandler slog.Handler
	if cfg.LogFormat == "json" {
		logHandler = slog.NewJSONHandler(os.Stdout, handlerOpts)
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, handlerOpts)
	}
	slog.SetDefault(slog.New(logHandler))

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil { // rwxr-xr-x: owner full, group+others read+execute
		slog.Error("failed to create data directory", "path", cfg.DataDir, "err", err)
		os.Exit(1)
	}

	metadataStore := store.New(cfg.DataDir)

	toEnqueue, err := recovery.Run(cfg.DataDir, metadataStore)
	if err != nil {
		slog.Error("startup recovery failed", "err", err)
		os.Exit(1)
	}

	jobQueue := worker.NewQueue(10000)
	processor := worker.NewProcessor(cfg.DataDir, cfg.DiskExpansionFactor, metadataStore)
	workerPool := worker.NewPool(jobQueue, processor)

	for _, id := range toEnqueue {
		jobQueue.Enqueue(id)
	}
	workerPool.Start(cfg.Workers)
	slog.Info("worker pool started", "workers", cfg.Workers, "queued_on_start", len(toEnqueue))

	cleaner := cleanup.New(cfg.DataDir, cfg.RetentionDays, cfg.CleanupInterval, metadataStore)
	cleaner.Start()

	handlerCfg := api.HandlerConfig{
		DataDir:        cfg.DataDir,
		MaxUploadBytes: cfg.MaxUploadBytes,
		BaseURL:        cfg.BaseURL,
		Workers:        cfg.Workers,
	}
	h := api.New(handlerCfg, metadataStore, jobQueue, workerPool, staticfiles.FS())
	router := api.NewRouter(h)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		slog.Info("PRS listening", "port", cfg.Port, "base_url", cfg.BaseURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-shutdownSignal
	slog.Info("shutting down...")

	// Give in-flight requests and workers up to 30 seconds to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(ctx)

	workerPool.Stop()
	cleaner.Stop()
	slog.Info("shutdown complete")
}
