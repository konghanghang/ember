-- Media gaps management
-- Changes:
-- 1) Add media_gaps table for independent missing-episode work orders
-- 2) Add unique key on tmdbId + season + episode
-- 3) Add status/airDate index for admin filtering and embySeriesId index for webhook fallback matching
-- Backfill:
-- - None. New table only.
-- Idempotency:
-- - Safe to run repeatedly via IF NOT EXISTS guards.

CREATE TABLE IF NOT EXISTS media_gaps (
  id varchar(25) PRIMARY KEY,
  "tmdbId" varchar(50) NOT NULL,
  "embySeriesId" varchar(50) NOT NULL DEFAULT '',
  "seriesName" varchar(255) NOT NULL DEFAULT '',
  season integer NOT NULL,
  episode integer NOT NULL,
  "airDate" timestamp with time zone NOT NULL,
  status varchar(20) NOT NULL DEFAULT 'MISSING',
  "searchSnapshot" text NOT NULL DEFAULT '',
  "dispatchSnapshot" text NOT NULL DEFAULT '',
  "lastScannedAt" timestamp with time zone,
  "lastSearchedAt" timestamp with time zone,
  "requestedAt" timestamp with time zone,
  "ingestedAt" timestamp with time zone,
  "ignoredAt" timestamp with time zone,
  "ignoreReason" text NOT NULL DEFAULT '',
  "createdAt" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updatedAt" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_media_gap_episode
  ON media_gaps ("tmdbId", season, episode);

CREATE INDEX IF NOT EXISTS idx_media_gaps_status_air_date
  ON media_gaps (status, "airDate");

CREATE INDEX IF NOT EXISTS idx_media_gaps_emby_series_id
  ON media_gaps ("embySeriesId");

CREATE INDEX IF NOT EXISTS idx_media_gaps_tmdb_id
  ON media_gaps ("tmdbId");
