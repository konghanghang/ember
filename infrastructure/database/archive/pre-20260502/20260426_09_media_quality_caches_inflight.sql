ALTER TABLE media_quality_caches
  ADD COLUMN IF NOT EXISTS "schema_version" integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS "inflight_until" timestamptz;
CREATE INDEX IF NOT EXISTS idx_media_quality_caches_inflight
  ON media_quality_caches ("inflight_until")
  WHERE "inflight_until" IS NOT NULL;
