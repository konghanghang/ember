ALTER TABLE tv_calendar_sources
  ADD COLUMN IF NOT EXISTS "lastFullSyncAt" timestamptz,
  ADD COLUMN IF NOT EXISTS "lastCorrectionAt" timestamptz;
