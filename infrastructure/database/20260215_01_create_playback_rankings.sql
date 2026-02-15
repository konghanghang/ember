-- Ember migration: create playback_rankings table
-- Date: 2026-02-15
--
-- Notes:
-- - Use CREATE ... IF NOT EXISTS to keep it safe when AUTO_MIGRATE is enabled in some environments.
-- - Timestamps use timestamptz to match GORM's default behavior with time.Time + UTC NowFunc.

CREATE TABLE IF NOT EXISTS playback_rankings (
  id           varchar(25)  PRIMARY KEY,
  period       varchar(10)  NOT NULL,
  category     varchar(20)  NOT NULL,
  rank         integer      NOT NULL,
  item_name    varchar(500) NOT NULL,
  play_count   integer      NOT NULL,
  duration     bigint       NOT NULL,
  snapshot_at  timestamptz  NOT NULL,
  period_start timestamptz  NOT NULL,
  period_end   timestamptz  NOT NULL,
  created_at   timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ranking_lookup
  ON playback_rankings (period, category, snapshot_at);

