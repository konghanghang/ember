ALTER TABLE media_quality_caches
  ADD COLUMN IF NOT EXISTS "schemaVersion" integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS "inflightUntil" timestamptz;
CREATE INDEX IF NOT EXISTS idx_media_quality_caches_inflight
  ON media_quality_caches ("inflightUntil")
  WHERE "inflightUntil" IS NOT NULL;
