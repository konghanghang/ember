-- 20260428_07_schema_alignment
-- 用途：清理重复 / 冗余索引，将 users.telegramId 改为 partial unique 索引，
--       清理 media_gaps 冗余 tmdbId 单列索引。
--
-- 改了哪些表、字段、索引、约束：
--   - users: 删除 inviteCode 列（如存在）及其索引；telegramId 改 partial unique
--   - media_gaps: 删除冗余的单列 idx_media_gaps_tmdb_id
--   - payments: 删除重复 idx_payments_stripe_session
--   - media_quality_caches: 删除重复 idx_media_quality_caches_library_id
--   - tmdb_cache: 删除重复 idx_tmdb_cache_cache_key
--   - tv_calendar_items: 删除重复 uk_tv_calendar_episode
--   - tv_calendar_sources: 删除重复 idx_tv_calendar_sources_tmdb_id
--   - tv_calendar_subscriptions: 删除重复 uk_tv_calendar_subscription
--   - client_blacklists: 删除重复 idx_client_blacklists_normalized_client_name
--
-- 是否需要回填：否
-- 是否可重复执行：是（DROP IF EXISTS + CREATE IF NOT EXISTS）

-- 清理重复索引（IF EXISTS 幂等）
DROP INDEX IF EXISTS idx_payments_stripe_session;
DROP INDEX IF EXISTS idx_media_quality_caches_library_id;
DROP INDEX IF EXISTS idx_tmdb_cache_cache_key;
DROP INDEX IF EXISTS uk_tv_calendar_episode;
DROP INDEX IF EXISTS idx_tv_calendar_sources_tmdb_id;
DROP INDEX IF EXISTS uk_tv_calendar_subscription;
DROP INDEX IF EXISTS idx_client_blacklists_normalized_client_name;

-- users.inviteCode 死字段（如果存在）
DROP INDEX IF EXISTS idx_users_invite_code;
ALTER TABLE users DROP COLUMN IF EXISTS "inviteCode";

-- users.telegramId 改 partial unique
-- 先删旧的全量 unique index（如存在），再建 partial unique
DROP INDEX IF EXISTS uk_users_telegram_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_telegram_id
  ON users USING btree ("telegramId")
  WHERE "telegramId" IS NOT NULL;

-- media_gaps tmdbId 冗余单列索引清理
-- 复合唯一索引 uk_media_gap_episode 已覆盖 tmdbId 的查询，单列索引冗余
DROP INDEX IF EXISTS idx_media_gaps_tmdb_id;
