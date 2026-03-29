-- Ember migration: add season column to subscriptions and upgrade unique index
-- Date: 2026-03-29
--
-- Purpose:
-- 1) Add `season` column to subscriptions (`0` means whole show)
-- 2) Backfill existing rows to `0`
-- 3) Replace unique index `(type, tmdbId)` with `(type, tmdbId, season)`
--
-- Notes:
-- - Script is idempotent.
-- - Existing movie subscriptions remain `season=0`.
-- - Existing TV subscriptions are backfilled to `season=0`.

BEGIN;

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS season integer;

UPDATE subscriptions
SET season = 0
WHERE season IS NULL;

ALTER TABLE subscriptions
  ALTER COLUMN season SET DEFAULT 0;

ALTER TABLE subscriptions
  ALTER COLUMN season SET NOT NULL;

DROP INDEX IF EXISTS uk_subscription_media;

CREATE UNIQUE INDEX IF NOT EXISTS uk_subscription_media
  ON subscriptions (type, "tmdbId", season);

COMMIT;
