package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/notifyhub/event-driven-arch/internal/domain"
	"github.com/notifyhub/event-driven-arch/internal/queue"
	"github.com/notifyhub/event-driven-arch/internal/repository"
)

// SchedulerWorker polls the database for notifications whose scheduled_at
// has passed and enqueues them for immediate processing.
//
// Notifications created with a future scheduled_at are stored with
// status=scheduled and bypass the queue until their time arrives.
type SchedulerWorker struct {
	repo     repository.NotificationRepository
	q        *queue.PriorityQueue
	interval time.Duration
	logger   *zap.Logger
}

func NewSchedulerWorker(
	repo repository.NotificationRepository,
	q *queue.PriorityQueue,
	interval time.Duration,
	logger *zap.Logger,
) *SchedulerWorker {
	return &SchedulerWorker{repo: repo, q: q, interval: interval, logger: logger}
}

// Run ticks every interval and enqueues any notifications that are now due.
// Stops cleanly when ctx is cancelled.
func (sw *SchedulerWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(sw.interval)
	defer ticker.Stop()

	sw.logger.Info("scheduler worker started", zap.Duration("interval", sw.interval))

	for {
		select {
		case <-ctx.Done():
			sw.logger.Info("scheduler worker stopping")
			return
		case <-ticker.C:
			sw.poll(ctx)
		}
	}
}

func (sw *SchedulerWorker) poll(ctx context.Context) {
	notifications, err := sw.repo.FindDueScheduled(ctx)
	if err != nil {
		sw.logger.Error("scheduler poll error", zap.Error(err))
		return
	}

	for _, n := range notifications {
		if err := sw.q.Enqueue(queue.Item{
			NotificationID: n.ID,
			Channel:        n.Channel,
			Priority:       n.Priority,
		}); err != nil {
			sw.logger.Warn("could not enqueue scheduled notification",
				zap.String("id", n.ID), zap.Error(err))
			continue
		}

		if err := sw.repo.UpdateStatus(ctx, n.ID, domain.StatusQueued); err != nil {
			sw.logger.Error("failed to update status after scheduling",
				zap.String("id", n.ID), zap.Error(err))
		}
	}

	if len(notifications) > 0 {
		sw.logger.Info("enqueued due scheduled notifications", zap.Int("count", len(notifications)))
	}
}
