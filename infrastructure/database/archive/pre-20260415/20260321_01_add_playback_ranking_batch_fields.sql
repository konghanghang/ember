-- Ember migration: add playback ranking batch and stable item fields
-- Date: 2026-03-21
--
-- Purpose:
-- 1) Add playback_rankings.batch_id so movie/episode rows from one generation share a stable batch
-- 2) Add playback_rankings.item_key and item_source_type for stable media aggregation
-- 3) Add new lookup indexes used by latest/history ranking queries
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing rows keep empty batch_id/item_key/item_source_type and are treated as legacy snapshots.
-- - This migration is required when AUTO_MIGRATE is disabled.

BEGIN;

ALTER TABLE playback_rankings
  ADD COLUMN IF NOT EXISTS batch_id varchar(25) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS item_key varchar(128) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS item_source_type varchar(32) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_ranking_batch
  ON playback_rankings (batch_id, category, rank);

CREATE INDEX IF NOT EXISTS idx_ranking_period_window
  ON playback_rankings (period, period_start, period_end, snapshot_at);

CREATE INDEX IF NOT EXISTS idx_ranking_item
  ON playback_rankings (period, category, item_key, period_start, period_end);

DROP INDEX IF EXISTS idx_ranking_lookup;

COMMIT;
