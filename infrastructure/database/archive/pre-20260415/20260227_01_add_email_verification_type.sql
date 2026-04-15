-- Ember migration: add email verification type for register/reset isolation
-- Date: 2026-02-27
--
-- Purpose:
-- 1) Add email_verifications.type to distinguish register and forgot-password code
-- 2) Add index for type-based filtering
--
-- Notes:
-- - Use default 'register' to keep backward compatibility for existing rows.
-- - Script is idempotent and safe to run multiple times.

BEGIN;

ALTER TABLE email_verifications
  ADD COLUMN IF NOT EXISTS "type" varchar(20) NOT NULL DEFAULT 'register';

CREATE INDEX IF NOT EXISTS idx_email_verifications_type
  ON email_verifications ("type");

COMMIT;
