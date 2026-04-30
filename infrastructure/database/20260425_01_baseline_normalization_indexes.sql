-- Backfill baseline normalization indexes for environments upgraded from pre-baseline migrations.
--
-- baseline `20260422_00_schema_baseline.sql` still carries two historical indexes that some upgraded databases may miss
-- from the legacy production source database (the dump was made before the model
-- definitions were tightened):
--
--   - idx_ranking_lookup        on playback_rankings(period, category, snapshot_at)
--   - uq_redemptions_user_code  unique on redemptions("userId", code)
--
-- Environments that initialised the database from the baseline file already carry
-- these indexes. But environments upgraded only via the post-baseline incremental
-- SQL never receive them, so VerifySchema would flag them missing once the index
-- fingerprints below are added.
--
-- Idempotency: CREATE [UNIQUE] INDEX IF NOT EXISTS keeps repeated runs safe; new
-- environments that already applied the baseline will turn this migration into a
-- no-op.

CREATE INDEX IF NOT EXISTS idx_ranking_lookup
  ON playback_rankings USING btree (period, category, snapshot_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_redemptions_user_code
  ON redemptions USING btree ("userId", code);
