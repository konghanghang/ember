-- Ember migration: add telegram binding support
-- Date: 2026-02-23
--
-- Purpose:
-- 1) Add users.telegramId for Telegram account binding
-- 2) Create telegram_bind_codes table for temporary bind codes
--
-- Notes:
-- - Column names use quoted camelCase to match GORM model tags.
-- - Script is idempotent and safe to run multiple times.

BEGIN;

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS "telegramId" bigint;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_id
  ON users ("telegramId")
  WHERE "telegramId" IS NOT NULL;

CREATE TABLE IF NOT EXISTS telegram_bind_codes (
  id          varchar(25)  PRIMARY KEY,
  "userId"    varchar(25)  NOT NULL,
  code        varchar(6)   NOT NULL,
  "expiresAt" timestamptz  NOT NULL,
  "createdAt" timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_telegram_bind_codes_user_id
  ON telegram_bind_codes ("userId");

-- 清理重复 code，保留每个 code 最新的一条，避免唯一索引创建失败
WITH duplicated AS (
  SELECT ctid,
         ROW_NUMBER() OVER (PARTITION BY code ORDER BY "createdAt" DESC, id DESC) AS rn
  FROM telegram_bind_codes
)
DELETE FROM telegram_bind_codes t
USING duplicated d
WHERE t.ctid = d.ctid
  AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_bind_codes_code
  ON telegram_bind_codes (code);

COMMIT;
