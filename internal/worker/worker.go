package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ricirt/event-driven-arch/internal/domain"
	"github.com/ricirt/event-driven-arch/internal/provider"
	"github.com/ricirt/event-driven-arch/internal/queue"
	"github.com/ricirt/event-driven-arch/internal/ratelimiter"
	"github.com/ricirt/event-driven-arch/internal/repository"
)

// Worker is a single goroutine that continuously pulls items from the priority
// queue, applies per-channel rate limiting, delivers via the provider, and
// handles retry scheduling on failure.
type Worker struct {
	id      int
	q       *queue.PriorityQueue
	repo    repository.NotificationRepository
	prov    provider.Provider
	limiter *ratelimiter.ChannelLimiters
	backoff []time.Duration
	logger  *zap.Logger

	// Hooks for metrics — injected by the pool so the worker stays metrics-agnostic.
	onSent    func(channel domain.Channel, latency time.Duration)
	onFailed  func(channel domain.Channel)
}

// NewWorker constructs a worker. onSent and onFailed are optional (nil = no-op).
func NewWorker(
	id int,
	q *queue.PriorityQueue,
	repo repository.NotificationRepository,
	prov provider.Provider,
	limiter *ratelimiter.ChannelLimiters,
	backoff []time.Duration,
	logger *zap.Logger,
	onSent func(domain.Channel, time.Duration),
	onFailed func(domain.Channel),
) *Worker {
	if onSent == nil {
		onSent = func(domain.Channel, time.Duration) {}
	}
	if onFailed == nil {
		onFailed = func(domain.Channel) {}
	}
	return &Worker{
		id: id, q: q, repo: repo, prov: prov,
		limiter: limiter, backoff: backoff, logger: logger,
		onSent: onSent, onFailed: onFailed,
	}
}

// Run blocks until ctx is cancelled, processing one queue item per iteration.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("worker started", zap.Int("id", w.id))
	for {
		item, ok := w.q.Dequeue(ctx)
		if !ok {
			w.logger.Info("worker stopping", zap.Int("id", w.id))
			return
		}
		w.process(ctx, item)
	}
}

func (w *Worker) process(ctx context.Context, item queue.Item) {
	start := time.Now()
	log := w.logger.With(
		zap.String("notification_id", item.NotificationID),
		zap.String("channel", string(item.Channel)),
	)

	n, err := w.repo.GetByID(ctx, item.NotificationID)
	if err != nil {
		log.Error("failed to fetch notification", zap.Error(err))
		return
	}

	// A cancellation between enqueue and processing time is valid; skip silently.
	if n.Status == domain.StatusCancelled {
		log.Debug("notification was cancelled before processing")
		return
	}

	if err := w.repo.UpdateStatus(ctx, n.ID, domain.StatusProcessing); err != nil {
		log.Error("failed to mark as processing", zap.Error(err))
		return
	}

	// Block here until the per-channel rate limiter grants a token.
	if err := w.limiter.Wait(ctx, n.Channel); err != nil {
		// ctx cancelled while waiting — worker is shutting down.
		return
	}

	resp, err := w.prov.Send(ctx, n)
	elapsed := time.Since(start)

	if err != nil {
		log.Warn("provider send failed",
			zap.Error(err),
			zap.Int("retry_count", n.RetryCount),
		)
		w.handleFailure(ctx, n, err)
		w.onFailed(n.Channel)
		return
	}

	now := time.Now().UTC()
	if err := w.repo.MarkSent(ctx, n.ID, resp.MessageID, now); err != nil {
		log.Error("failed to mark as sent", zap.Error(err))
		return
	}

	// Update batch counters asynchronously if this notification belongs to a batch.
	if n.BatchID != nil {
		go func() {
			if err := w.repo.UpdateBatchCounts(context.Background(), *n.BatchID); err != nil {
				log.Warn("failed to update batch counts", zap.Error(err))
			}
		}()
	}

	w.onSent(n.Channel, elapsed)
	log.Info("notification sent", zap.String("provider_msg_id", resp.MessageID), zap.Duration("latency", elapsed))
}

// handleFailure either schedules a retry (if retries remain) or marks the
// notification as permanently failed.
//
// Retry schedule uses exponential backoff:
//
//	attempt 0 → backoff[0]  (default 5 s)
//	attempt 1 → backoff[1]  (default 30 s)
//	attempt 2 → backoff[2]  (default 120 s)
//	attempt N ≥ len(backoff) → last backoff entry (clamped)
func (w *Worker) handleFailure(ctx context.Context, n *domain.Notification, sendErr error) {
	if n.RetryCount >= n.MaxRetries {
		if err := w.repo.MarkFailed(ctx, n.ID, sendErr.Error()); err != nil {
			w.logger.Error("failed to mark notification as failed",
				zap.String("id", n.ID), zap.Error(err))
		}
		return
	}

	idx := n.RetryCount
	if idx >= len(w.backoff) {
		idx = len(w.backoff) - 1
	}
	nextRetry := time.Now().UTC().Add(w.backoff[idx])

	if err := w.repo.ScheduleRetry(ctx, n.ID, n.RetryCount+1, nextRetry, sendErr.Error()); err != nil {
		w.logger.Error("failed to schedule retry",
			zap.String("id", n.ID), zap.Error(err))
	}
}
