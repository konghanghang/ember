-- Ember migration: add tv calendar source activity timestamp
-- Date: 2026-03-14
--
-- Purpose:
-- 1) Add tv_calendar_sources."lastEpisodeIngestedAt"
-- 2) Add index for incremental active-source sync
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing rows keep NULL and will be backfilled by later sync/webhook activity.
-- - This migration is required when AUTO_MIGRATE is disabled.

BEGIN;

ALTER TABLE tv_calendar_sources
  ADD COLUMN IF NOT EXISTS "lastEpisodeIngestedAt" timestamptz;

CREATE INDEX IF NOT EXISTS idx_tv_calendar_sources_last_episode_ingested_at
  ON tv_calendar_sources ("lastEpisodeIngestedAt");

COMMIT;
