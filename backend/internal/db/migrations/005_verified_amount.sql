-- Migration 005: Add verified_amount_cents for operator amount resolution on mismatched deposits.
-- The operator sets this when approving an amount_mismatch flagged deposit.
-- Once set, it replaces amount_cents as the ledger posting amount.

ALTER TABLE transfers ADD COLUMN IF NOT EXISTS verified_amount_cents BIGINT;
