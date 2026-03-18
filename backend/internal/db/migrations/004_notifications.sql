CREATE TABLE IF NOT EXISTS notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  VARCHAR(50) NOT NULL REFERENCES accounts(id),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    type        VARCHAR(30) NOT NULL,
    title       VARCHAR(200) NOT NULL,
    message     TEXT NOT NULL,
    metadata    JSONB NOT NULL DEFAULT '{}',
    read        BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_account_id ON notifications(account_id);
CREATE INDEX IF NOT EXISTS idx_notifications_account_unread ON notifications(account_id, read) WHERE read = false;
CREATE INDEX IF NOT EXISTS idx_notifications_transfer_id ON notifications(transfer_id);
