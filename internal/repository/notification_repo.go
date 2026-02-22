package repository

import (
	"context"
	"time"

	"github.com/ricirt/event-driven-arch/internal/domain"
)

// NotificationRepository defines all persistence operations for notifications.
// The pgx implementation is in pg_notification_repo.go.
// Tests use a hand-written mock (mock_notification_repo.go).
type NotificationRepository interface {
	Create(ctx context.Context, n *domain.Notification) error
	GetByID(ctx context.Context, id string) (*domain.Notification, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Notification, error)
	List(ctx context.Context, filter domain.ListFilter) ([]*domain.Notification, int, error)
	UpdateStatus(ctx context.Context, id string, status domain.Status) error
	MarkSent(ctx context.Context, id string, providerMsgID string, sentAt time.Time) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
	ScheduleRetry(ctx context.Context, id string, retryCount int, nextRetry time.Time, errMsg string) error
	Cancel(ctx context.Context, id string) error
	FindDueRetries(ctx context.Context) ([]*domain.Notification, error)
	FindDueScheduled(ctx context.Context) ([]*domain.Notification, error)

	CreateBatch(ctx context.Context, batchID string, notifications []*domain.Notification) (*domain.Batch, error)
	GetBatch(ctx context.Context, batchID string) (*domain.Batch, []*domain.Notification, error)
	UpdateBatchCounts(ctx context.Context, batchID string) error
}
