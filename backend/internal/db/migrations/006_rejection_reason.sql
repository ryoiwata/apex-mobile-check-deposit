-- Migration 006: Add rejection_reason to transfers
-- Stores the human-readable reason a transfer was rejected (vendor or operator).

ALTER TABLE transfers ADD COLUMN IF NOT EXISTS rejection_reason TEXT;
