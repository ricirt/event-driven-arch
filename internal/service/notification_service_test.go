package service_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/notifyhub/event-driven-arch/internal/domain"
	"github.com/notifyhub/event-driven-arch/internal/queue"
	"github.com/notifyhub/event-driven-arch/internal/repository"
	"github.com/notifyhub/event-driven-arch/internal/service"
)

func newService() (*service.NotificationService, *repository.MockNotificationRepository, *queue.PriorityQueue) {
	repo := repository.NewMockNotificationRepository()
	q := queue.New()
	svc := service.NewNotificationService(repo, q, zap.NewNop())
	return svc, repo, q
}

var validReq = domain.CreateNotificationRequest{
	Channel:   domain.ChannelSMS,
	Recipient: "+905551234567",
	Content:   "Test message",
	Priority:  domain.PriorityNormal,
}

func TestNotificationService_Create(t *testing.T) {
	svc, _, q := newService()
	ctx := context.Background()

	n, isDuplicate, err := svc.Create(ctx, validReq, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDuplicate {
		t.Fatal("expected isDuplicate=false for a new notification")
	}
	if n.ID == "" {
		t.Fatal("expected a non-empty ID")
	}
	if n.Status != domain.StatusQueued {
		t.Fatalf("expected status=queued, got %s", n.Status)
	}

	high, normal, low := q.Depths()
	if high+normal+low == 0 {
		t.Fatal("expected item to be enqueued")
	}
}

func TestNotificationService_Create_InvalidRequest(t *testing.T) {
	svc, _, _ := newService()

	bad := validReq
	bad.Channel = "fax"
	_, _, err := svc.Create(context.Background(), bad, "")
	if err != domain.ErrInvalidChannel {
		t.Fatalf("expected ErrInvalidChannel, got %v", err)
	}
}

func TestNotificationService_Create_IdempotencyReturnsDuplicate(t *testing.T) {
	svc, _, _ := newService()
	ctx := context.Background()

	key := "idem-key-123"
	first, isDup, err := svc.Create(ctx, validReq, key)
	if err != nil || isDup {
		t.Fatalf("first call: err=%v isDup=%v", err, isDup)
	}

	second, isDup, err := svc.Create(ctx, validReq, key)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if !isDup {
		t.Fatal("expected isDuplicate=true for repeated idempotency key")
	}
	if second.ID != first.ID {
		t.Fatal("expected same notification ID on duplicate")
	}
}

func TestNotificationService_Cancel_States(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		status      domain.Status
		expectedErr error
	}{
		{"pending can be cancelled", domain.StatusPending, nil},
		{"queued can be cancelled", domain.StatusQueued, nil},
		{"already cancelled", domain.StatusCancelled, domain.ErrAlreadyCancelled},
		{"processing cannot be cancelled", domain.StatusProcessing, domain.ErrNotCancellable},
		{"sent cannot be cancelled", domain.StatusSent, domain.ErrNotCancellable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newService()

			n, _, _ := svc.Create(ctx, validReq, "")
			_ = repo.UpdateStatus(ctx, n.ID, tc.status)

			err := svc.Cancel(ctx, n.ID)
			if err != tc.expectedErr {
				t.Fatalf("expected %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestNotificationService_Cancel_NotFound(t *testing.T) {
	svc, _, _ := newService()
	err := svc.Cancel(context.Background(), "nonexistent-id")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNotificationService_CreateBatch(t *testing.T) {
	svc, _, _ := newService()

	requests := make([]domain.CreateNotificationRequest, 5)
	for i := range requests {
		requests[i] = validReq
	}

	batch, err := svc.CreateBatch(context.Background(), requests)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if batch.Total != 5 {
		t.Fatalf("expected total=5, got %d", batch.Total)
	}
}

func TestNotificationService_CreateBatch_TooLarge(t *testing.T) {
	svc, _, _ := newService()

	requests := make([]domain.CreateNotificationRequest, 1001)
	for i := range requests {
		requests[i] = validReq
	}

	_, err := svc.CreateBatch(context.Background(), requests)
	if err != domain.ErrBatchTooLarge {
		t.Fatalf("expected ErrBatchTooLarge, got %v", err)
	}
}

func TestNotificationService_CreateBatch_Empty(t *testing.T) {
	svc, _, _ := newService()
	_, err := svc.CreateBatch(context.Background(), nil)
	if err != domain.ErrBatchEmpty {
		t.Fatalf("expected ErrBatchEmpty, got %v", err)
	}
}

func TestNotificationService_GetByID(t *testing.T) {
	svc, _, _ := newService()
	ctx := context.Background()

	n, _, _ := svc.Create(ctx, validReq, "")

	got, err := svc.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != n.ID {
		t.Fatalf("expected id=%s, got %s", n.ID, got.ID)
	}
}

func TestNotificationService_GetByID_NotFound(t *testing.T) {
	svc, _, _ := newService()
	_, err := svc.GetByID(context.Background(), "does-not-exist")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
