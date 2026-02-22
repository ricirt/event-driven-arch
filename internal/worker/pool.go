package worker

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ricirt/event-driven-arch/internal/config"
	"github.com/ricirt/event-driven-arch/internal/domain"
	"github.com/ricirt/event-driven-arch/internal/provider"
	"github.com/ricirt/event-driven-arch/internal/queue"
	"github.com/ricirt/event-driven-arch/internal/ratelimiter"
	"github.com/ricirt/event-driven-arch/internal/repository"
)

// MetricHooks carries the metric callback functions injected by main.
// Using a struct keeps the pool constructor signature clean.
type MetricHooks struct {
	OnSent   func(channel domain.Channel, latency time.Duration)
	OnFailed func(channel domain.Channel)
}

// Pool manages the lifecycle of all workers.
// All workers share the same priority queue — the queue's double-select
// pattern handles priority ordering internally.
type Pool struct {
	workers []*Worker
	wg      sync.WaitGroup
}

// NewPool creates (SMS + Email + Push) workers as configured.
// All workers are identical; the channel type distinction is handled
// by the rate limiter and the notification's Channel field.
func NewPool(
	cfg *config.Config,
	q *queue.PriorityQueue,
	repo repository.NotificationRepository,
	prov provider.Provider,
	limiter *ratelimiter.ChannelLimiters,
	logger *zap.Logger,
	hooks MetricHooks,
) *Pool {
	total := cfg.SMSWorkers + cfg.EmailWorkers + cfg.PushWorkers
	workers := make([]*Worker, total)

	for i := range workers {
		workers[i] = NewWorker(
			i, q, repo, prov, limiter,
			cfg.RetryBackoff,
			logger.With(zap.Int("worker_id", i)),
			hooks.OnSent,
			hooks.OnFailed,
		)
	}

	return &Pool{workers: workers}
}

// Start launches all workers as goroutines.
// The provided ctx is forwarded to every worker; cancelling it
// triggers a graceful shutdown of the entire pool.
func (p *Pool) Start(ctx context.Context) {
	for _, w := range p.workers {
		p.wg.Add(1)
		go func(w *Worker) {
			defer p.wg.Done()
			w.Run(ctx)
		}(w)
	}
}

// Wait blocks until every worker has returned after ctx is cancelled.
// Call this after cancelling the context to ensure in-flight messages finish.
func (p *Pool) Wait() {
	p.wg.Wait()
}
