-- Batches group notifications created together.
-- Created before notifications because notifications.batch_id references this table.

CREATE TABLE batches (
    id         TEXT        PRIMARY KEY,
    total      INT         NOT NULL DEFAULT 0,
    pending    INT         NOT NULL DEFAULT 0,
    sent       INT         NOT NULL DEFAULT 0,
    failed     INT         NOT NULL DEFAULT 0,
    cancelled  INT         NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Auto-update updated_at on every row change.
-- Defined once here; reused by the notifications trigger.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_batches_updated_at
    BEFORE UPDATE ON batches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
