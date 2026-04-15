-- Ember migration: 新增媒体库质量缓存表
-- Date: 2026-03-06
--
-- Purpose:
-- 1) 新增 media_quality_caches（按媒体库维度缓存质量报告）
--
-- Notes:
-- - 脚本幂等，可重复执行。
-- - 字段名采用 camelCase，需双引号包裹。

BEGIN;

CREATE TABLE IF NOT EXISTS media_quality_caches (
  id           varchar(25)  PRIMARY KEY,
  "libraryId"  varchar(100) NOT NULL,
  statistics   text         NOT NULL,
  "expiresAt"  timestamptz  NOT NULL,
  "createdAt"  timestamptz  NOT NULL DEFAULT now(),
  "updatedAt"  timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_media_quality_caches_library_id
  ON media_quality_caches ("libraryId");

CREATE INDEX IF NOT EXISTS idx_media_quality_caches_expires_at
  ON media_quality_caches ("expiresAt");

COMMIT;
