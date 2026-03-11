-- Ember migration: 新增追剧日历相关表
-- Date: 2026-03-05
--
-- Purpose:
-- 1) 新增 tv_calendar_sources（全局追剧源）
-- 2) 新增 tv_calendar_items（全局追剧日历条目）
-- 3) 新增 tv_calendar_subscriptions（用户关注剧集）
-- 4) 新增 tmdb_cache（TMDB 持久化缓存）
--
-- Notes:
-- - 脚本幂等，可重复执行。
-- - 字段名采用 camelCase，需双引号包裹。
-- - 该脚本为未上线版本的最终结构，不做历史兼容补丁。

BEGIN;

CREATE TABLE IF NOT EXISTS tv_calendar_sources (
  id             varchar(25)  PRIMARY KEY,
  "tmdbId"       varchar(50)  NOT NULL,
  "seriesId"     varchar(50)  NOT NULL DEFAULT '',
  "showName"     varchar(255) NOT NULL DEFAULT '',
  "posterUrl"    varchar(500) NOT NULL DEFAULT '',
  overview       text         NOT NULL DEFAULT '',
  "embyStatus"   varchar(20)  NOT NULL DEFAULT 'continuing',
  "lastSyncedAt" timestamptz,
  "createdAt"    timestamptz  NOT NULL DEFAULT now(),
  "updatedAt"    timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tv_calendar_sources_tmdb_id
  ON tv_calendar_sources ("tmdbId");

CREATE INDEX IF NOT EXISTS idx_tv_calendar_sources_series_id
  ON tv_calendar_sources ("seriesId");

CREATE TABLE IF NOT EXISTS tv_calendar_items (
  id            varchar(25)  PRIMARY KEY,
  "tmdbId"      varchar(50)  NOT NULL,
  "seriesId"    varchar(50)  NOT NULL DEFAULT '',
  season        integer      NOT NULL,
  episode       integer      NOT NULL,
  "airDate"     timestamptz  NOT NULL,
  "episodeName" varchar(255) NOT NULL DEFAULT '',
  overview      text         NOT NULL DEFAULT '',
  status        varchar(20)  NOT NULL DEFAULT 'upcoming',
  "embyItemId"  varchar(50)  NOT NULL DEFAULT '',
  "lastChecked" timestamptz  NOT NULL DEFAULT now(),
  "createdAt"   timestamptz  NOT NULL DEFAULT now(),
  "updatedAt"   timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tv_calendar_items_tmdb_season_episode
  ON tv_calendar_items ("tmdbId", season, episode);

CREATE INDEX IF NOT EXISTS idx_tv_calendar_items_tmdb_id
  ON tv_calendar_items ("tmdbId");

CREATE INDEX IF NOT EXISTS idx_tv_calendar_items_series_id
  ON tv_calendar_items ("seriesId");

CREATE INDEX IF NOT EXISTS idx_tv_calendar_items_air_date
  ON tv_calendar_items ("airDate");

CREATE TABLE IF NOT EXISTS tv_calendar_subscriptions (
  id          varchar(25)  PRIMARY KEY,
  "userId"    varchar(25)  NOT NULL,
  "tmdbId"    varchar(50)  NOT NULL,
  "showName"  varchar(255) NOT NULL,
  "posterUrl" varchar(500) NOT NULL DEFAULT '',
  "createdAt" timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tv_calendar_subscriptions_user_tmdb
  ON tv_calendar_subscriptions ("userId", "tmdbId");

CREATE INDEX IF NOT EXISTS idx_tv_calendar_subscriptions_user_id
  ON tv_calendar_subscriptions ("userId");

CREATE INDEX IF NOT EXISTS idx_tv_calendar_subscriptions_tmdb_id
  ON tv_calendar_subscriptions ("tmdbId");

CREATE TABLE IF NOT EXISTS tmdb_cache (
  id           varchar(25)  PRIMARY KEY,
  "cacheKey"   varchar(255) NOT NULL,
  "cacheValue" text         NOT NULL,
  "expiresAt"  timestamptz  NOT NULL,
  "createdAt"  timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tmdb_cache_cache_key
  ON tmdb_cache ("cacheKey");

CREATE INDEX IF NOT EXISTS idx_tmdb_cache_expires_at
  ON tmdb_cache ("expiresAt");

COMMIT;
