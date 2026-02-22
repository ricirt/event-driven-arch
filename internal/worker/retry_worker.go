package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ricirt/event-driven-arch/internal/domain"
	"github.com/ricirt/event-driven-arch/internal/queue"
	"github.com/ricirt/event-driven-arch/internal/repository"
)

// RetryWorker polls the database for failed notifications whose
// next_retry_at is in the past and re-enqueues them.
//
// This DB-backed approach means retries survive server restarts:
// scheduled retry times are persisted, not held in memory.
type RetryWorker struct {
	repo     repository.NotificationRepository
	q        *queue.PriorityQueue
	interval time.Duration
	logger   *zap.Logger
}

func NewRetryWorker(
	repo repository.NotificationRepository,
	q *queue.PriorityQueue,
	interval time.Duration,
	logger *zap.Logger,
) *RetryWorker {
	return &RetryWorker{repo: repo, q: q, interval: interval, logger: logger}
}

// Run ticks every interval and re-enqueues any due retries.
// Stops cleanly when ctx is cancelled.
func (rw *RetryWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(rw.interval)
	defer ticker.Stop()

	rw.logger.Info("retry worker started", zap.Duration("interval", rw.interval))

	for {
		select {
		case <-ctx.Done():
			rw.logger.Info("retry worker stopping")
			return
		case <-ticker.C:
			rw.poll(ctx)
		}
	}
}

func (rw *RetryWorker) poll(ctx context.Context) {
	notifications, err := rw.repo.FindDueRetries(ctx)
	if err != nil {
		rw.logger.Error("retry poll error", zap.Error(err))
		return
	}

	for _, n := range notifications {
		if err := rw.q.Enqueue(queue.Item{
			NotificationID: n.ID,
			Channel:        n.Channel,
			Priority:       n.Priority,
		}); err != nil {
			rw.logger.Warn("could not re-enqueue retry",
				zap.String("id", n.ID), zap.Error(err))
			continue
		}

		if err := rw.repo.UpdateStatus(ctx, n.ID, domain.StatusQueued); err != nil {
			rw.logger.Error("failed to update status after re-enqueue",
				zap.String("id", n.ID), zap.Error(err))
		}
	}

	if len(notifications) > 0 {
		rw.logger.Info("re-enqueued due retries", zap.Int("count", len(notifications)))
	}
}
