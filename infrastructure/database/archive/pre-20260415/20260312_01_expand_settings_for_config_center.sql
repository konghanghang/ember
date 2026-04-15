-- Ember migration: expand settings table for config center
-- Date: 2026-03-12
--
-- Purpose:
-- 1) Expand settings.key from varchar(50) to varchar(100)
-- 2) Expand settings.value from varchar(500) to text
-- 3) Add settings."isEncrypted" for sensitive config storage
-- 4) Add settings."updatedByUserId" for last modifier tracking
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing data is preserved.
-- - This migration is required when AUTO_MIGRATE is disabled.

BEGIN;

ALTER TABLE settings
  ALTER COLUMN key TYPE varchar(100);

ALTER TABLE settings
  ALTER COLUMN value TYPE text;

ALTER TABLE settings
  ADD COLUMN IF NOT EXISTS "isEncrypted" boolean;

UPDATE settings
SET "isEncrypted" = false
WHERE "isEncrypted" IS NULL;

ALTER TABLE settings
  ALTER COLUMN "isEncrypted" SET DEFAULT false;

ALTER TABLE settings
  ALTER COLUMN "isEncrypted" SET NOT NULL;

ALTER TABLE settings
  ADD COLUMN IF NOT EXISTS "updatedByUserId" varchar(25);

COMMIT;
