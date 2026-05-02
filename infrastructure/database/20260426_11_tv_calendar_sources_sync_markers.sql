ALTER TABLE tv_calendar_sources
  ADD COLUMN IF NOT EXISTS "last_full_sync_at" timestamptz,
  ADD COLUMN IF NOT EXISTS "last_correction_at" timestamptz;
