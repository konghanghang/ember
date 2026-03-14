-- Ember migration: add payment checkout url
-- Date: 2026-03-14
--
-- Purpose:
-- 1) Add payments."checkoutUrl" for pending order reuse
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing rows keep empty string and will be reused only after new checkout creation.
-- - This migration is required when AUTO_MIGRATE is disabled.

BEGIN;

ALTER TABLE payments
  ADD COLUMN IF NOT EXISTS "checkoutUrl" varchar(2048) NOT NULL DEFAULT '';

COMMIT;
