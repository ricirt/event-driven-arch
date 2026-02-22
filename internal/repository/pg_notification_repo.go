package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ricirt/event-driven-arch/internal/domain"
)

type pgNotificationRepository struct {
	pool *pgxpool.Pool
}

// NewPgNotificationRepository returns a NotificationRepository backed by PostgreSQL.
func NewPgNotificationRepository(pool *pgxpool.Pool) NotificationRepository {
	return &pgNotificationRepository{pool: pool}
}

func (r *pgNotificationRepository) Create(ctx context.Context, n *domain.Notification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notifications
			(id, batch_id, channel, recipient, content, priority, status,
			 idempotency_key, retry_count, max_retries, scheduled_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		n.ID, n.BatchID, n.Channel, n.Recipient, n.Content, n.Priority, n.Status,
		n.IdempotencyKey, n.RetryCount, n.MaxRetries, n.ScheduledAt, n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "idempotency_key") {
			return domain.ErrConflict
		}
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func (r *pgNotificationRepository) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, batch_id, channel, recipient, content, priority, status,
		       idempotency_key, retry_count, max_retries, next_retry_at,
		       scheduled_at, sent_at, provider_msg_id, error_message,
		       created_at, updated_at
		FROM notifications WHERE id = $1`, id)

	n, err := scanNotification(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return n, err
}

func (r *pgNotificationRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Notification, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, batch_id, channel, recipient, content, priority, status,
		       idempotency_key, retry_count, max_retries, next_retry_at,
		       scheduled_at, sent_at, provider_msg_id, error_message,
		       created_at, updated_at
		FROM notifications WHERE idempotency_key = $1`, key)

	n, err := scanNotification(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return n, err
}

func (r *pgNotificationRepository) List(ctx context.Context, f domain.ListFilter) ([]*domain.Notification, int, error) {
	where, args := buildListWhere(f)
	offset := (f.Page - 1) * f.Limit

	// Count total matching rows for pagination metadata.
	var total int
	countQuery := "SELECT COUNT(*) FROM notifications" + where
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	// Append pagination args after the WHERE args.
	args = append(args, f.Limit, offset)
	limitPlaceholder := fmt.Sprintf("$%d", len(args)-1)
	offsetPlaceholder := fmt.Sprintf("$%d", len(args))

	query := fmt.Sprintf(`
		SELECT id, batch_id, channel, recipient, content, priority, status,
		       idempotency_key, retry_count, max_retries, next_retry_at,
		       scheduled_at, sent_at, provider_msg_id, error_message,
		       created_at, updated_at
		FROM notifications%s
		ORDER BY created_at DESC
		LIMIT %s OFFSET %s`, where, limitPlaceholder, offsetPlaceholder)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}
	return notifications, total, rows.Err()
}

func (r *pgNotificationRepository) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status = $1 WHERE id = $2`, status, id)
	return err
}

func (r *pgNotificationRepository) MarkSent(ctx context.Context, id, providerMsgID string, sentAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications
		SET status = 'sent', provider_msg_id = $1, sent_at = $2, error_message = NULL
		WHERE id = $3`, providerMsgID, sentAt, id)
	return err
}

func (r *pgNotificationRepository) MarkFailed(ctx context.Context, id, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications
		SET status = 'failed', error_message = $1, next_retry_at = NULL
		WHERE id = $2`, errMsg, id)
	return err
}

func (r *pgNotificationRepository) ScheduleRetry(ctx context.Context, id string, retryCount int, nextRetry time.Time, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications
		SET status = 'failed', retry_count = $1, next_retry_at = $2, error_message = $3
		WHERE id = $4`, retryCount, nextRetry, errMsg, id)
	return err
}

func (r *pgNotificationRepository) Cancel(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status = 'cancelled' WHERE id = $1`, id)
	return err
}

func (r *pgNotificationRepository) FindDueRetries(ctx context.Context) ([]*domain.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, batch_id, channel, recipient, content, priority, status,
		       idempotency_key, retry_count, max_retries, next_retry_at,
		       scheduled_at, sent_at, provider_msg_id, error_message,
		       created_at, updated_at
		FROM notifications
		WHERE status = 'failed'
		  AND retry_count < max_retries
		  AND next_retry_at <= NOW()
		LIMIT 500`)
	if err != nil {
		return nil, fmt.Errorf("find due retries: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func (r *pgNotificationRepository) FindDueScheduled(ctx context.Context) ([]*domain.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, batch_id, channel, recipient, content, priority, status,
		       idempotency_key, retry_count, max_retries, next_retry_at,
		       scheduled_at, sent_at, provider_msg_id, error_message,
		       created_at, updated_at
		FROM notifications
		WHERE status = 'scheduled'
		  AND scheduled_at <= NOW()
		LIMIT 500`)
	if err != nil {
		return nil, fmt.Errorf("find due scheduled: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func (r *pgNotificationRepository) CreateBatch(ctx context.Context, batchID string, notifications []*domain.Notification) (*domain.Batch, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &domain.Batch{
		ID:        batchID,
		Total:     len(notifications),
		Pending:   len(notifications),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO batches (id, total, pending, sent, failed, cancelled, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		batch.ID, batch.Total, batch.Pending, 0, 0, 0, batch.CreatedAt, batch.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert batch: %w", err)
	}

	for _, n := range notifications {
		_, err = tx.Exec(ctx, `
			INSERT INTO notifications
				(id, batch_id, channel, recipient, content, priority, status,
				 idempotency_key, retry_count, max_retries, scheduled_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			n.ID, n.BatchID, n.Channel, n.Recipient, n.Content, n.Priority, n.Status,
			n.IdempotencyKey, n.RetryCount, n.MaxRetries, n.ScheduledAt, n.CreatedAt, n.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert batch notification: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit batch: %w", err)
	}

	return batch, nil
}

func (r *pgNotificationRepository) GetBatch(ctx context.Context, batchID string) (*domain.Batch, []*domain.Notification, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, total, pending, sent, failed, cancelled, created_at, updated_at
		FROM batches WHERE id = $1`, batchID)

	var b domain.Batch
	err := row.Scan(&b.ID, &b.Total, &b.Pending, &b.Sent, &b.Failed, &b.Cancelled, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get batch: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, batch_id, channel, recipient, content, priority, status,
		       idempotency_key, retry_count, max_retries, next_retry_at,
		       scheduled_at, sent_at, provider_msg_id, error_message,
		       created_at, updated_at
		FROM notifications WHERE batch_id = $1 ORDER BY created_at ASC`, batchID)
	if err != nil {
		return nil, nil, fmt.Errorf("get batch notifications: %w", err)
	}
	defer rows.Close()

	notifications, err := scanNotifications(rows)
	return &b, notifications, err
}

func (r *pgNotificationRepository) UpdateBatchCounts(ctx context.Context, batchID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE batches b
		SET
			pending   = (SELECT COUNT(*) FROM notifications WHERE batch_id = b.id AND status IN ('pending','queued','processing','scheduled')),
			sent      = (SELECT COUNT(*) FROM notifications WHERE batch_id = b.id AND status = 'sent'),
			failed    = (SELECT COUNT(*) FROM notifications WHERE batch_id = b.id AND status = 'failed'),
			cancelled = (SELECT COUNT(*) FROM notifications WHERE batch_id = b.id AND status = 'cancelled')
		WHERE id = $1`, batchID)
	return err
}

// ---- helpers ----

// scanNotification reads a single notification row from any pgx row type.
func scanNotification(row pgx.Row) (*domain.Notification, error) {
	var n domain.Notification
	err := row.Scan(
		&n.ID, &n.BatchID, &n.Channel, &n.Recipient, &n.Content,
		&n.Priority, &n.Status, &n.IdempotencyKey,
		&n.RetryCount, &n.MaxRetries, &n.NextRetryAt,
		&n.ScheduledAt, &n.SentAt, &n.ProviderMsgID, &n.ErrorMessage,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func scanNotifications(rows pgx.Rows) ([]*domain.Notification, error) {
	var result []*domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// buildListWhere builds a parameterised WHERE clause from a ListFilter.
func buildListWhere(f domain.ListFilter) (string, []any) {
	var conditions []string
	var args []any

	add := func(condition string, val any) {
		args = append(args, val)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}

	if f.Status != nil {
		add("status = $%d", *f.Status)
	}
	if f.Channel != nil {
		add("channel = $%d", *f.Channel)
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("created_at <= $%d", *f.To)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
