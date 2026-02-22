CREATE TYPE notification_channel AS ENUM ('sms', 'email', 'push');
CREATE TYPE notification_priority AS ENUM ('high', 'normal', 'low');
CREATE TYPE notification_status AS ENUM (
    'pending',
    'queued',
    'processing',
    'sent',
    'failed',
    'cancelled',
    'scheduled'
);

CREATE TABLE notifications (
    id              TEXT                  PRIMARY KEY,
    batch_id        TEXT                  REFERENCES batches(id) ON DELETE SET NULL,
    channel         notification_channel  NOT NULL,
    recipient       TEXT                  NOT NULL,
    content         TEXT                  NOT NULL,
    priority        notification_priority NOT NULL DEFAULT 'normal',
    status          notification_status   NOT NULL DEFAULT 'pending',
    idempotency_key TEXT                  UNIQUE,
    retry_count     SMALLINT              NOT NULL DEFAULT 0,
    max_retries     SMALLINT              NOT NULL DEFAULT 3,
    next_retry_at   TIMESTAMPTZ,
    scheduled_at    TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    provider_msg_id TEXT,
    error_message   TEXT,
    created_at      TIMESTAMPTZ           NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ           NOT NULL DEFAULT NOW()
);

-- Common query patterns
CREATE INDEX idx_notifications_status     ON notifications(status);
CREATE INDEX idx_notifications_channel    ON notifications(channel);
CREATE INDEX idx_notifications_batch_id   ON notifications(batch_id);
CREATE INDEX idx_notifications_created_at ON notifications(created_at DESC);

-- Partial index: retry worker only touches failed rows that still have retries left
CREATE INDEX idx_notifications_retry ON notifications(next_retry_at)
    WHERE status = 'failed' AND retry_count < max_retries;

-- Partial index: scheduler worker only touches rows awaiting their scheduled time
CREATE INDEX idx_notifications_scheduled ON notifications(scheduled_at)
    WHERE status = 'scheduled';

CREATE TRIGGER trg_notifications_updated_at
    BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
