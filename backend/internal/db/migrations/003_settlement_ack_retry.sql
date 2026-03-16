-- Add retry tracking columns to settlement_batches
ALTER TABLE settlement_batches
    ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_retry_at TIMESTAMPTZ;

-- Extend the status check constraint to include retry/escalation states.
-- Postgres auto-names inline CHECK constraints as <table>_<column>_check.
ALTER TABLE settlement_batches DROP CONSTRAINT IF EXISTS settlement_batches_status_check;

ALTER TABLE settlement_batches
    ADD CONSTRAINT settlement_batches_status_check
    CHECK (status IN ('pending','submitted','acknowledged','retry_pending','escalated'));
