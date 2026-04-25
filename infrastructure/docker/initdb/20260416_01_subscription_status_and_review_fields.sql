-- Subscription status visibility and review metadata
-- Changes:
-- 1) Add subscriptions.rejectReason for admin rejection reason
-- 2) Add subscriptions.reviewedAt for approval/rejection timestamp
-- 3) Add subscriptions.ingestedAt for real Emby ingestion timestamp
-- 4) Expand allowed status values to include INGESTED (column is varchar, no enum alter required)
-- Backfill:
-- - None. Existing rows keep NULL review/ingest metadata.
-- Idempotency:
-- - Safe to run repeatedly via IF NOT EXISTS guards.

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS "rejectReason" text;

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS "reviewedAt" timestamp with time zone;

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS "ingestedAt" timestamp with time zone;
