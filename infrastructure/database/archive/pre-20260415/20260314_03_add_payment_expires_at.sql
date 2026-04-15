-- Ember migration: add payment expiration timestamp
-- Date: 2026-03-14
--
-- Purpose:
-- 1) Add payments."expiresAt" for local pending-order expiration control
-- 2) Backfill existing pending rows with createdAt + 30 minutes
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing non-pending rows keep NULL because they no longer need local expiration control.
-- - This migration is required when AUTO_MIGRATE is disabled.

BEGIN;

ALTER TABLE payments
  ADD COLUMN IF NOT EXISTS "expiresAt" timestamptz;

CREATE INDEX IF NOT EXISTS idx_payments_expires_at
  ON payments ("expiresAt");

UPDATE payments
SET "expiresAt" = "createdAt" + interval '30 minutes'
WHERE status = 'pending'
  AND "expiresAt" IS NULL;

COMMIT;
