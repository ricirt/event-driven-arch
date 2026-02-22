package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/ricirt/event-driven-arch/internal/api"
	"github.com/ricirt/event-driven-arch/internal/config"
	"github.com/ricirt/event-driven-arch/internal/db"
	"github.com/ricirt/event-driven-arch/internal/metrics"
	"github.com/ricirt/event-driven-arch/internal/provider"
	"github.com/ricirt/event-driven-arch/internal/queue"
	"github.com/ricirt/event-driven-arch/internal/ratelimiter"
	"github.com/ricirt/event-driven-arch/internal/repository"
	"github.com/ricirt/event-driven-arch/internal/service"
	"github.com/ricirt/event-driven-arch/internal/worker"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() //nolint:errcheck

	// ---- configuration ----
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// ---- database ----
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	logger.Info("database migrations applied")

	// ---- core dependencies ----
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	q := queue.New()
	repo := repository.NewPgNotificationRepository(pool)
	prov := provider.NewWebhookProvider(cfg.ProviderBaseURL, cfg.ProviderTimeout)
	limiter := ratelimiter.New(cfg.RateLimit)
	svc := service.NewNotificationService(repo, q, logger)

	// ---- worker pool ----
	// Context for all background goroutines; cancelled on shutdown signal.
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	onSent, onFailed := m.WorkerHooks()
	pool2 := worker.NewPool(cfg, q, repo, prov, limiter, logger, worker.MetricHooks{
		OnSent:   onSent,
		OnFailed: onFailed,
	})
	pool2.Start(workerCtx)

	retryW := worker.NewRetryWorker(repo, q, cfg.RetryInterval, logger)
	go retryW.Run(workerCtx)

	schedulerW := worker.NewSchedulerWorker(repo, q, cfg.SchedulerInterval, logger)
	go schedulerW.Run(workerCtx)

	// ---- HTTP server ----
	router := api.NewRouter(svc, q, reg, logger)
	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Start server in a goroutine so it does not block the shutdown listener.
	go func() {
		logger.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// ---- graceful shutdown ----
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received")

	// 1. Stop accepting new HTTP requests.
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}

	// 2. Signal all workers to stop processing new queue items.
	cancelWorkers()

	// 3. Wait for in-flight workers to finish their current message.
	pool2.Wait()

	logger.Info("server stopped cleanly")
}
