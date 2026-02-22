-- Ember migration: add email verification support
-- Date: 2026-02-22
--
-- Purpose:
-- 1) Create email_verifications table for code verification and rate limiting
-- 2) Seed settings.email_verification = false (if missing)
--
-- Notes:
-- - Column names "expiresAt"/"createdAt"/"updatedAt" use quoted camelCase to match current GORM model tags.
-- - Script is idempotent and safe to run multiple times.

BEGIN;

CREATE TABLE IF NOT EXISTS email_verifications (
  id          varchar(25)  PRIMARY KEY,
  email       varchar(255) NOT NULL,
  code        varchar(6)   NOT NULL,
  ip          varchar(45)  NOT NULL,
  "expiresAt" timestamptz  NOT NULL,
  "createdAt" timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_verifications_email
  ON email_verifications (email);

CREATE INDEX IF NOT EXISTS idx_email_verifications_ip
  ON email_verifications (ip);

INSERT INTO settings ("key", "value", "updatedAt")
VALUES ('email_verification', 'false', now())
ON CONFLICT ("key") DO NOTHING;

COMMIT;
