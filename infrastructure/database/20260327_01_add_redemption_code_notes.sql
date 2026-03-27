-- Ember migration: add notes column to redemption_codes
-- Date: 2026-03-27
--
-- Purpose:
-- 1) Add `notes` column (varchar 500) to redemption_codes
-- 2) Keep existing rows compatible (default empty string)

-- Notes:
-- - Script is idempotent.
-- - `notes` allows empty string and is not used for filtering.

BEGIN;

ALTER TABLE redemption_codes
  ADD COLUMN IF NOT EXISTS notes varchar(500) NOT NULL DEFAULT '';

COMMIT;
