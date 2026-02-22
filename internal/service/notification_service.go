package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/notifyhub/event-driven-arch/internal/domain"
	"github.com/notifyhub/event-driven-arch/internal/queue"
	"github.com/notifyhub/event-driven-arch/internal/repository"
)

// NotificationService coordinates the repository and queue.
// All business rules (idempotency, cancel state machine, batch limits) live here.
// HTTP handlers and workers depend on this service, not on each other.
type NotificationService struct {
	repo   repository.NotificationRepository
	q      *queue.PriorityQueue
	logger *zap.Logger
}

func NewNotificationService(
	repo repository.NotificationRepository,
	q *queue.PriorityQueue,
	logger *zap.Logger,
) *NotificationService {
	return &NotificationService{repo: repo, q: q, logger: logger}
}

// Create validates, persists, and enqueues a single notification.
//
// Idempotency: if an X-Idempotency-Key header was supplied and a notification
// with that key already exists, the existing record is returned as-is.
// The caller can distinguish a repeat response by the HTTP status code
// (200 for existing, 201 for newly created).
func (s *NotificationService) Create(
	ctx context.Context,
	req domain.CreateNotificationRequest,
	idempotencyKey string,
) (*domain.Notification, bool, error) {
	if err := req.Validate(); err != nil {
		return nil, false, err
	}

	// --- idempotency check ---
	if idempotencyKey != "" {
		existing, err := s.repo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, false, fmt.Errorf("idempotency lookup: %w", err)
		}
		if existing != nil {
			return existing, true, nil // true = was a duplicate
		}
	}

	n := s.buildNotification(req, idempotencyKey, nil)

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, false, fmt.Errorf("persist notification: %w", err)
	}

	s.enqueue(ctx, n)
	return n, false, nil
}

// CreateBatch validates and creates up to 1000 notifications in a single
// transaction, then enqueues the non-scheduled ones.
func (s *NotificationService) CreateBatch(
	ctx context.Context,
	requests []domain.CreateNotificationRequest,
) (*domain.Batch, error) {
	if len(requests) == 0 {
		return nil, domain.ErrBatchEmpty
	}
	if len(requests) > 1000 {
		return nil, domain.ErrBatchTooLarge
	}

	batchID := uuid.New().String()
	now := time.Now().UTC()

	notifications := make([]*domain.Notification, len(requests))
	for i, req := range requests {
		if err := req.Validate(); err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		notifications[i] = s.buildNotification(req, "", &batchID)
		notifications[i].CreatedAt = now
		notifications[i].UpdatedAt = now
	}

	batch, err := s.repo.CreateBatch(ctx, batchID, notifications)
	if err != nil {
		return nil, fmt.Errorf("persist batch: %w", err)
	}

	for _, n := range notifications {
		if n.ScheduledAt == nil {
			s.enqueue(ctx, n)
		}
	}

	return batch, nil
}

// Cancel marks a notification as cancelled if it is still in a cancellable state.
func (s *NotificationService) Cancel(ctx context.Context, id string) error {
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	switch n.Status {
	case domain.StatusCancelled:
		return domain.ErrAlreadyCancelled
	case domain.StatusProcessing, domain.StatusSent:
		return domain.ErrNotCancellable
	}

	return s.repo.Cancel(ctx, id)
}

func (s *NotificationService) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *NotificationService) List(ctx context.Context, filter domain.ListFilter) ([]*domain.Notification, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *NotificationService) GetBatch(ctx context.Context, batchID string) (*domain.Batch, []*domain.Notification, error) {
	return s.repo.GetBatch(ctx, batchID)
}

// ---- private helpers ----

func (s *NotificationService) buildNotification(
	req domain.CreateNotificationRequest,
	idempotencyKey string,
	batchID *string,
) *domain.Notification {
	now := time.Now().UTC()
	status := domain.StatusPending
	if req.ScheduledAt != nil {
		status = domain.StatusScheduled
	}

	n := &domain.Notification{
		ID:         uuid.New().String(),
		BatchID:    batchID,
		Channel:    req.Channel,
		Recipient:  req.Recipient,
		Content:    req.Content,
		Priority:   req.Priority,
		Status:     status,
		MaxRetries: 3,
		ScheduledAt: req.ScheduledAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if idempotencyKey != "" {
		n.IdempotencyKey = &idempotencyKey
	}

	return n
}

// enqueue places the notification on the queue and updates its status to queued.
// If the queue is full the notification remains in status=pending; the retry
// worker will not re-enqueue pending items, so for robustness a separate
// recovery mechanism (or operator alert on queue_depth gauges) is warranted
// in production. For this scope we log a warning.
func (s *NotificationService) enqueue(ctx context.Context, n *domain.Notification) {
	if n.ScheduledAt != nil {
		return // scheduler worker handles these
	}

	if err := s.q.Enqueue(queue.Item{
		NotificationID: n.ID,
		Channel:        n.Channel,
		Priority:       n.Priority,
	}); err != nil {
		s.logger.Warn("queue full: notification will remain pending",
			zap.String("id", n.ID), zap.Error(err))
		return
	}

	if err := s.repo.UpdateStatus(ctx, n.ID, domain.StatusQueued); err != nil {
		s.logger.Error("failed to update status to queued", zap.String("id", n.ID), zap.Error(err))
		return
	}
	n.Status = domain.StatusQueued
}
