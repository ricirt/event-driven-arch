package queue

import (
	"context"
	"fmt"

	"github.com/ricirt/event-driven-arch/internal/domain"
)

// PriorityQueue dispatches items to one of three buffered channels based on priority.
//
// Buffer sizes reflect expected traffic ratios:
//
//	High:   1 000  — must never accumulate; small buffer applies back-pressure quickly
//	Normal: 5 000  — bulk of traffic
//	Low:    2 000  — background / best-effort
//
// Workers dequeue via the double-select pattern, which guarantees that
// high-priority items are always served before normal or low ones, while
// still allowing fair competition between normal and low when high is empty.
type PriorityQueue struct {
	high   chan Item
	normal chan Item
	low    chan Item
}

func New() *PriorityQueue {
	return &PriorityQueue{
		high:   make(chan Item, 1000),
		normal: make(chan Item, 5000),
		low:    make(chan Item, 2000),
	}
}

// Enqueue places an item on the appropriate priority channel.
// It is non-blocking: if the target channel is full, ErrQueueFull is returned
// immediately rather than blocking the caller (the HTTP handler).
func (q *PriorityQueue) Enqueue(item Item) error {
	switch item.Priority {
	case domain.PriorityHigh:
		select {
		case q.high <- item:
			return nil
		default:
			return domain.ErrQueueFull
		}
	case domain.PriorityNormal:
		select {
		case q.normal <- item:
			return nil
		default:
			return domain.ErrQueueFull
		}
	case domain.PriorityLow:
		select {
		case q.low <- item:
			return nil
		default:
			return domain.ErrQueueFull
		}
	default:
		return fmt.Errorf("unknown priority %q", item.Priority)
	}
}

// Dequeue blocks until an item is available or ctx is cancelled.
//
// Priority guarantee — the double-select pattern:
//  1. A non-blocking select checks the high channel first. If an item is
//     waiting there, it is returned immediately regardless of normal/low.
//  2. Only when high is empty does the goroutine enter a fair blocking select
//     across all three channels plus the done signal. This prevents high-priority
//     starvation while still letting the worker sleep instead of spinning.
//
// Returns (Item{}, false) when ctx is cancelled (graceful shutdown signal).
func (q *PriorityQueue) Dequeue(ctx context.Context) (Item, bool) {
	// Step 1: drain high before entering a fair wait.
	select {
	case item := <-q.high:
		return item, true
	default:
	}

	// Step 2: fair competition when high is empty.
	select {
	case item := <-q.high:
		return item, true
	case item := <-q.normal:
		return item, true
	case item := <-q.low:
		return item, true
	case <-ctx.Done():
		return Item{}, false
	}
}

// Depths returns the current number of items waiting in each priority tier.
// Used by the metrics handler for the queue-depth snapshot.
func (q *PriorityQueue) Depths() (high, normal, low int) {
	return len(q.high), len(q.normal), len(q.low)
}
