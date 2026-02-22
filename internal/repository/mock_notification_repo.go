package repository

import (
	"context"
	"sync"
	"time"

	"github.com/ricirt/event-driven-arch/internal/domain"
)

// MockNotificationRepository is a hand-written, in-memory implementation of
// NotificationRepository used in unit tests. No mock-generation library needed.
type MockNotificationRepository struct {
	mu            sync.RWMutex
	notifications map[string]*domain.Notification
	batches       map[string]*domain.Batch

	// Optional error overrides — set in tests to simulate failure paths.
	CreateErr              error
	GetByIDErr             error
	GetByIdempotencyKeyErr error
}

func NewMockNotificationRepository() *MockNotificationRepository {
	return &MockNotificationRepository{
		notifications: make(map[string]*domain.Notification),
		batches:       make(map[string]*domain.Batch),
	}
}

func (m *MockNotificationRepository) Create(_ context.Context, n *domain.Notification) error {
	if m.CreateErr != nil {
		return m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if n.IdempotencyKey != nil {
		for _, existing := range m.notifications {
			if existing.IdempotencyKey != nil && *existing.IdempotencyKey == *n.IdempotencyKey {
				return domain.ErrConflict
			}
		}
	}
	clone := *n
	m.notifications[n.ID] = &clone
	return nil
}

func (m *MockNotificationRepository) GetByID(_ context.Context, id string) (*domain.Notification, error) {
	if m.GetByIDErr != nil {
		return nil, m.GetByIDErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.notifications[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := *n
	return &clone, nil
}

func (m *MockNotificationRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.Notification, error) {
	if m.GetByIdempotencyKeyErr != nil {
		return nil, m.GetByIdempotencyKeyErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, n := range m.notifications {
		if n.IdempotencyKey != nil && *n.IdempotencyKey == key {
			clone := *n
			return &clone, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *MockNotificationRepository) List(_ context.Context, _ domain.ListFilter) ([]*domain.Notification, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*domain.Notification, 0, len(m.notifications))
	for _, n := range m.notifications {
		clone := *n
		result = append(result, &clone)
	}
	return result, len(result), nil
}

func (m *MockNotificationRepository) UpdateStatus(_ context.Context, id string, status domain.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		n.Status = status
	}
	return nil
}

func (m *MockNotificationRepository) MarkSent(_ context.Context, id, providerMsgID string, sentAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		n.Status = domain.StatusSent
		n.ProviderMsgID = &providerMsgID
		n.SentAt = &sentAt
	}
	return nil
}

func (m *MockNotificationRepository) MarkFailed(_ context.Context, id, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		n.Status = domain.StatusFailed
		n.ErrorMessage = &errMsg
	}
	return nil
}

func (m *MockNotificationRepository) ScheduleRetry(_ context.Context, id string, retryCount int, nextRetry time.Time, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		n.RetryCount = retryCount
		n.NextRetryAt = &nextRetry
		n.ErrorMessage = &errMsg
		n.Status = domain.StatusFailed
	}
	return nil
}

func (m *MockNotificationRepository) Cancel(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		n.Status = domain.StatusCancelled
	}
	return nil
}

func (m *MockNotificationRepository) FindDueRetries(_ context.Context) ([]*domain.Notification, error) {
	return nil, nil
}

func (m *MockNotificationRepository) FindDueScheduled(_ context.Context) ([]*domain.Notification, error) {
	return nil, nil
}

func (m *MockNotificationRepository) CreateBatch(_ context.Context, batchID string, notifications []*domain.Notification) (*domain.Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	batch := &domain.Batch{
		ID:        batchID,
		Total:     len(notifications),
		Pending:   len(notifications),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	m.batches[batchID] = batch
	for _, n := range notifications {
		clone := *n
		m.notifications[n.ID] = &clone
	}
	return batch, nil
}

func (m *MockNotificationRepository) GetBatch(_ context.Context, batchID string) (*domain.Batch, []*domain.Notification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.batches[batchID]
	if !ok {
		return nil, nil, domain.ErrNotFound
	}
	var notifications []*domain.Notification
	for _, n := range m.notifications {
		if n.BatchID != nil && *n.BatchID == batchID {
			clone := *n
			notifications = append(notifications, &clone)
		}
	}
	batchClone := *b
	return &batchClone, notifications, nil
}

func (m *MockNotificationRepository) UpdateBatchCounts(_ context.Context, _ string) error {
	return nil
}
