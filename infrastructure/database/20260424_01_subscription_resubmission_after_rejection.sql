-- Subscription resubmission after rejection
-- Changes:
-- 1) Add subscriptions.retryFromId to link a resubmission to the rejected source record
-- 2) Drop the old global unique index uk_subscription_media
-- 3) Add a lookup index for subscriptions.retryFromId
-- 4) Add an active-state partial unique index for type + tmdbId + season
-- Backfill:
-- - None. Existing rows keep NULL retryFromId.
-- Idempotency:
-- - Safe to run repeatedly via IF NOT EXISTS / IF EXISTS guards.

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS "retryFromId" varchar(25);

DROP INDEX IF EXISTS uk_subscription_media;

CREATE INDEX IF NOT EXISTS idx_subscriptions_retry_from_id
  ON subscriptions ("retryFromId");

CREATE UNIQUE INDEX IF NOT EXISTS uq_subscriptions_active_media
  ON subscriptions (type, "tmdbId", season)
  WHERE status IN ('PENDING', 'APPROVED', 'INGESTED');
