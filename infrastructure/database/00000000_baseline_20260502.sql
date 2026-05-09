-- 20260502_00_schema_baseline.sql
--
-- v1.4.0 截点合并 baseline（cutoff: 2026-05-02）
--
-- 本文件是 v1.4.0 上线后唯一的"新装库初始化入口"。
-- 内容按字典序合并自下面 24 份历史顶层 SQL：
--   - 20260422_00_schema_baseline.sql（v1.3.1 截点 baseline）
--   - 20260423_00_..._snake_case.sql 至 20260427_04_..._reject_message_context.sql 共 23 个增量
-- 等价于在新装空库上按字典序逐个执行这 24 个文件的结果。
--
-- 与"严格 schema-only baseline"的差异：
--   - 保留各历史 SQL 的 DO 块、IF NOT EXISTS、historical DML 清洗语句
--   - 这些 DML 仅清洗历史脏数据（重复行、NULL 回填），新装空库上均为 no-op
--   - 选择此形式是为了在没有 PG 实例时也能直接维护 baseline
--
-- 历史增量 SQL 同步整批归档到 archive/pre-20260502/，
-- 仅供追溯，不再属于现行执行链路。
--
-- 各历史 SQL 内容以 `-- ┌─ <filename>` / `-- └─ <filename>` 边界注释包裹，
-- 便于追溯具体语句来自哪个原始迁移。


-- ┌─ 20260422_00_schema_baseline.sql ─────────────────────────────────────────────────
--
-- PostgreSQL database dump
--


-- Dumped from database version 15.15
-- Dumped by pg_dump version 18.0

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', 'public', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: media_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE "media_type" AS ENUM (
    'MOVIE',
    'TV'
);


--
-- Name: subscription_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE "subscription_status" AS ENUM (
    'PENDING',
    'APPROVED',
    'REJECTED',
    'INGESTED'
);


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: client_blacklists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE client_blacklists (
    id character varying(25) NOT NULL,
    "client_name" character varying(100) NOT NULL,
    "normalized_client_name" character varying(100) NOT NULL,
    reason character varying(255) DEFAULT ''::character varying,
    "created_at" timestamp with time zone DEFAULT now()
);


--
-- Name: device_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE device_actions (
    id character varying(25) NOT NULL,
    "device_id" character varying(100) DEFAULT ''::character varying,
    "user_id" character varying(25) DEFAULT ''::character varying,
    "client_name" character varying(100) DEFAULT ''::character varying,
    action character varying(50) NOT NULL,
    note character varying(255) DEFAULT ''::character varying,
    "created_at" timestamp with time zone DEFAULT now()
);


--
-- Name: email_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE email_verifications (
    id character varying(25) NOT NULL,
    email character varying(255) NOT NULL,
    code character varying(6) NOT NULL,
    ip character varying(45) NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT now(),
    type character varying(20) DEFAULT 'register'::character varying NOT NULL
);


--
-- Name: media_gaps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE media_gaps (
    id character varying(25) NOT NULL,
    "tmdb_id" character varying(50) NOT NULL,
    "emby_series_id" character varying(50) DEFAULT ''::character varying NOT NULL,
    "series_name" character varying(255) DEFAULT ''::character varying NOT NULL,
    season integer NOT NULL,
    episode integer NOT NULL,
    "air_date" timestamp with time zone NOT NULL,
    status character varying(20) DEFAULT 'MISSING'::character varying NOT NULL,
    "search_snapshot" text DEFAULT ''::text NOT NULL,
    "dispatch_snapshot" text DEFAULT ''::text NOT NULL,
    "last_scanned_at" timestamp with time zone,
    "last_searched_at" timestamp with time zone,
    "requested_at" timestamp with time zone,
    "ingested_at" timestamp with time zone,
    "ignored_at" timestamp with time zone,
    "ignore_reason" text DEFAULT ''::text NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: media_quality_caches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE media_quality_caches (
    id character varying(25) NOT NULL,
    "library_id" character varying(100) NOT NULL,
    statistics text NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT now(),
    "updated_at" timestamp with time zone DEFAULT now()
);


--
-- Name: payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payments (
    id character varying(25) NOT NULL,
    "user_id" character varying(25) NOT NULL,
    "plan_id" character varying(25) NOT NULL,
    "stripe_session_id" character varying(255) NOT NULL,
    "stripe_payment_intent_id" character varying(255) DEFAULT ''::character varying,
    amount bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    days bigint NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT now(),
    "updated_at" timestamp with time zone DEFAULT now(),
    "checkout_url" character varying(2048) DEFAULT ''::character varying NOT NULL,
    "expires_at" timestamp with time zone
);


--
-- Name: plan_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE plan_groups (
    key character varying(50) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(255) DEFAULT ''::character varying,
    "is_default" boolean DEFAULT false NOT NULL,
    "sort_order" integer DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now(),
    "updated_at" timestamp with time zone DEFAULT now()
);


--
-- Name: plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE plans (
    id character varying(25) NOT NULL,
    name character varying(100) NOT NULL,
    days bigint NOT NULL,
    price bigint NOT NULL,
    description character varying(255),
    "is_active" boolean DEFAULT true NOT NULL,
    "created_at" timestamp with time zone DEFAULT now(),
    "updated_at" timestamp with time zone DEFAULT now(),
    "plan_group" character varying(50) DEFAULT ''::character varying NOT NULL
);


--
-- Name: playback_rankings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE playback_rankings (
    id character varying(25) NOT NULL,
    period character varying(10) NOT NULL,
    category character varying(10) NOT NULL,
    rank bigint NOT NULL,
    item_name character varying(255) NOT NULL,
    item_type character varying(20) DEFAULT ''::character varying NOT NULL,
    item_key character varying(255) NOT NULL,
    metric_value bigint NOT NULL,
    duration bigint NOT NULL,
    user_count bigint NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    batch_id character varying(64) NOT NULL,
    snapshot_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: redemption_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE redemption_codes (
    id character varying(25) NOT NULL,
    code character varying(50) NOT NULL,
    days integer NOT NULL,
    max_uses integer DEFAULT 1 NOT NULL,
    "used_count" integer DEFAULT 0 NOT NULL,
    "is_active" boolean DEFAULT true NOT NULL,
    "expires_at" timestamp with time zone,
    "created_at" timestamp with time zone,
    "template_user_id" character varying(25),
    note text DEFAULT ''::text NOT NULL,
    "registration_plan_group" character varying(50) DEFAULT ''::character varying NOT NULL
);


--
-- Name: redemptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE redemptions (
    id character varying(25) NOT NULL,
    "user_id" character varying(25) NOT NULL,
    code character varying(50) NOT NULL,
    days integer NOT NULL,
    "redeemed_at" timestamp with time zone,
    "old_expiry_date" timestamp with time zone,
    "new_expiry_date" timestamp with time zone
);


--
-- Name: settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE settings (
    key character varying(100) NOT NULL,
    value text NOT NULL,
    "updated_at" timestamp with time zone,
    "is_encrypted" boolean DEFAULT false NOT NULL,
    "updated_by_user_id" character varying(25)
);


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE subscriptions (
    id character varying(25) NOT NULL,
    "user_id" character varying(25) NOT NULL,
    type character varying(10) NOT NULL,
    name character varying(255) NOT NULL,
    "tmdb_id" character varying(50) NOT NULL,
    "poster_path" character varying(500),
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    note text,
    "mp_error" character varying(500),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    season integer DEFAULT 0 NOT NULL,
    "reject_reason" text,
    "reviewed_at" timestamp with time zone,
    "ingested_at" timestamp with time zone
);


--
-- Name: telegram_bind_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE telegram_bind_codes (
    id character varying(25) NOT NULL,
    "user_id" character varying(25) NOT NULL,
    code character varying(6) NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT now()
);


--
-- Name: tmdb_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tmdb_cache (
    id character varying(25) NOT NULL,
    "cache_key" character varying(255) NOT NULL,
    "cache_value" text NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT now()
);


--
-- Name: tv_calendar_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tv_calendar_items (
    id character varying(25) NOT NULL,
    "tmdb_id" character varying(50) NOT NULL,
    "series_id" character varying(50) DEFAULT ''::character varying,
    season bigint NOT NULL,
    episode bigint NOT NULL,
    "air_date" timestamp with time zone NOT NULL,
    "episode_name" character varying(255) DEFAULT ''::character varying,
    status character varying(20) DEFAULT 'upcoming'::character varying NOT NULL,
    "emby_item_id" character varying(50) DEFAULT ''::character varying,
    "last_checked" timestamp with time zone DEFAULT now(),
    "created_at" timestamp with time zone DEFAULT now(),
    "updated_at" timestamp with time zone DEFAULT now(),
    overview text DEFAULT ''::text NOT NULL
);


--
-- Name: tv_calendar_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tv_calendar_sources (
    id character varying(25) NOT NULL,
    "tmdb_id" character varying(50) NOT NULL,
    "series_id" character varying(50) DEFAULT ''::character varying NOT NULL,
    "show_name" character varying(255) DEFAULT ''::character varying NOT NULL,
    "poster_url" character varying(500) DEFAULT ''::character varying NOT NULL,
    overview text DEFAULT ''::text NOT NULL,
    "emby_status" character varying(20) DEFAULT 'continuing'::character varying NOT NULL,
    "last_synced_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT now(),
    "updated_at" timestamp with time zone DEFAULT now(),
    "last_episode_ingested_at" timestamp with time zone
);


--
-- Name: tv_calendar_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tv_calendar_subscriptions (
    id character varying(25) NOT NULL,
    "user_id" character varying(25) NOT NULL,
    "tmdb_id" character varying(50) NOT NULL,
    "show_name" character varying(255) NOT NULL,
    "poster_url" character varying(500) DEFAULT ''::character varying,
    "created_at" timestamp with time zone DEFAULT now()
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE users (
    id character varying(25) NOT NULL,
    username character varying(50) NOT NULL,
    role character varying(10) DEFAULT 'user'::character varying NOT NULL,
    email character varying(255),
    password character varying(255) NOT NULL,
    "emby_id" character varying(255),
    "emby_username" character varying(255),
    "expiry_date" timestamp with time zone,
    "is_active" boolean DEFAULT true NOT NULL,
    "invite_code" character varying(50),
    "plan_group" character varying(50),
    "telegram_id" bigint,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: client_blacklists client_blacklists_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY client_blacklists
    ADD CONSTRAINT client_blacklists_pkey PRIMARY KEY (id);


--
-- Name: device_actions device_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY device_actions
    ADD CONSTRAINT device_actions_pkey PRIMARY KEY (id);


--
-- Name: email_verifications email_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);


--
-- Name: media_gaps media_gaps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_gaps
    ADD CONSTRAINT media_gaps_pkey PRIMARY KEY (id);


--
-- Name: media_quality_caches media_quality_caches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_quality_caches
    ADD CONSTRAINT media_quality_caches_pkey PRIMARY KEY (id);


--
-- Name: payments payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (id);


--
-- Name: plan_groups plan_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY plan_groups
    ADD CONSTRAINT plan_groups_pkey PRIMARY KEY (key);


--
-- Name: plans plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);


--
-- Name: playback_rankings playback_rankings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY playback_rankings
    ADD CONSTRAINT playback_rankings_pkey PRIMARY KEY (id);


--
-- Name: redemption_codes redemption_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY redemption_codes
    ADD CONSTRAINT redemption_codes_pkey PRIMARY KEY (id);


--
-- Name: redemptions redemptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY redemptions
    ADD CONSTRAINT redemptions_pkey PRIMARY KEY (id);


--
-- Name: settings settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: telegram_bind_codes telegram_bind_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY telegram_bind_codes
    ADD CONSTRAINT telegram_bind_codes_pkey PRIMARY KEY (id);


--
-- Name: tmdb_cache tmdb_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tmdb_cache
    ADD CONSTRAINT tmdb_cache_pkey PRIMARY KEY (id);


--
-- Name: tv_calendar_items tv_calendar_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tv_calendar_items
    ADD CONSTRAINT tv_calendar_items_pkey PRIMARY KEY (id);


--
-- Name: tv_calendar_sources tv_calendar_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tv_calendar_sources
    ADD CONSTRAINT tv_calendar_sources_pkey PRIMARY KEY (id);


--
-- Name: tv_calendar_subscriptions tv_calendar_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tv_calendar_subscriptions
    ADD CONSTRAINT tv_calendar_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: idx_email_verifications_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_created_at ON email_verifications USING btree ("created_at");


--
-- Name: idx_email_verifications_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_email ON email_verifications USING btree (email);


--
-- Name: idx_email_verifications_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_expires_at ON email_verifications USING btree ("expires_at");


--
-- Name: idx_media_gaps_emby_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_gaps_emby_series_id ON media_gaps USING btree ("emby_series_id");


--
-- Name: idx_media_gaps_status_air_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_gaps_status_air_date ON media_gaps USING btree (status, "air_date");


--
-- Name: idx_media_gaps_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_gaps_tmdb_id ON media_gaps USING btree ("tmdb_id");


--
-- Name: idx_payments_stripe_session; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payments_stripe_session ON payments USING btree ("stripe_session_id");


--
-- Name: idx_plans_is_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_is_active ON plans USING btree ("is_active");


--
-- Name: idx_plans_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_plan_group ON plans USING btree ("plan_group");


--
-- Name: idx_ranking_batch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ranking_batch ON playback_rankings USING btree (batch_id, category, rank);


--
-- Name: idx_ranking_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ranking_item ON playback_rankings USING btree (period, category, item_key, period_start, period_end);


--
-- Name: idx_ranking_period_window; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ranking_period_window ON playback_rankings USING btree (period, period_start, period_end, snapshot_at);


--
-- Name: idx_redemption_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_redemption_codes_code ON redemption_codes USING btree (code);


--
-- Name: idx_redemption_codes_registration_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemption_codes_registration_plan_group ON redemption_codes USING btree ("registration_plan_group");


--
-- Name: idx_redemption_codes_template_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemption_codes_template_user_id ON redemption_codes USING btree ("template_user_id");


--
-- Name: idx_redemptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemptions_user_id ON redemptions USING btree ("user_id");


--
-- Name: idx_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_user_id ON subscriptions USING btree ("user_id");


--
-- Name: idx_telegram_bind_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_telegram_bind_codes_code ON telegram_bind_codes USING btree (code);


--
-- Name: idx_telegram_bind_codes_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_bind_codes_user_id ON telegram_bind_codes USING btree ("user_id");


--
-- Name: idx_tmdb_cache_cache_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_tmdb_cache_cache_key ON tmdb_cache USING btree ("cache_key");


--
-- Name: idx_tmdb_cache_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tmdb_cache_expires_at ON tmdb_cache USING btree ("expires_at");


--
-- Name: idx_tv_calendar_items_air_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_air_date ON tv_calendar_items USING btree ("air_date");


--
-- Name: idx_tv_calendar_items_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_series_id ON tv_calendar_items USING btree ("series_id");


--
-- Name: idx_tv_calendar_items_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_tmdb_id ON tv_calendar_items USING btree ("tmdb_id");


--
-- Name: idx_tv_calendar_sources_last_episode_ingested_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_sources_last_episode_ingested_at ON tv_calendar_sources USING btree ("last_episode_ingested_at");


--
-- Name: idx_tv_calendar_sources_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_sources_series_id ON tv_calendar_sources USING btree ("series_id");


--
-- Name: idx_tv_calendar_sources_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_tv_calendar_sources_tmdb_id ON tv_calendar_sources USING btree ("tmdb_id");


--
-- Name: idx_tv_calendar_subscriptions_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_subscriptions_tmdb_id ON tv_calendar_subscriptions USING btree ("tmdb_id");


--
-- Name: idx_tv_calendar_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_subscriptions_user_id ON tv_calendar_subscriptions USING btree ("user_id");


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_email ON users USING btree (email);


--
-- Name: idx_users_emby_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_emby_id ON users USING btree ("emby_id");


--
-- Name: idx_users_invite_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_invite_code ON users USING btree ("invite_code");


--
-- Name: idx_users_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_plan_group ON users USING btree ("plan_group");


--
-- Name: idx_users_telegram_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_telegram_id ON users USING btree ("telegram_id") WHERE ("telegram_id" IS NOT NULL);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_username ON users USING btree (username);


--
-- Name: uk_media_gap_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_media_gap_episode ON media_gaps USING btree ("tmdb_id", season, episode);


--
-- Name: uk_subscription_media; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_subscription_media ON subscriptions USING btree (type, "tmdb_id", season);


--
-- Name: uk_tv_calendar_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_tv_calendar_episode ON tv_calendar_items USING btree ("tmdb_id", season, episode);


--
-- Name: uk_tv_calendar_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_tv_calendar_subscription ON tv_calendar_subscriptions USING btree ("user_id", "tmdb_id");


--
-- Name: uq_client_blacklists_normalized_client_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_client_blacklists_normalized_client_name ON client_blacklists USING btree ("normalized_client_name");


--
-- Name: uq_media_quality_caches_library_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_media_quality_caches_library_id ON media_quality_caches USING btree ("library_id");


--
-- Name: uq_plan_groups_default_true; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_plan_groups_default_true ON plan_groups USING btree ("is_default") WHERE ("is_default" = true);


--
-- Name: uq_tmdb_cache_cache_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tmdb_cache_cache_key ON tmdb_cache USING btree ("cache_key");


--
-- Name: uq_tv_calendar_items_tmdb_season_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_items_tmdb_season_episode ON tv_calendar_items USING btree ("tmdb_id", season, episode);


--
-- Name: uq_tv_calendar_sources_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_sources_tmdb_id ON tv_calendar_sources USING btree ("tmdb_id");


--
-- Name: uq_tv_calendar_subscriptions_user_tmdb; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_subscriptions_user_tmdb ON tv_calendar_subscriptions USING btree ("user_id", "tmdb_id");


--
-- PostgreSQL database dump complete
--



-- Ember baseline normalization: deterministic bootstrap data and missing migration indexes.

INSERT INTO settings (key, value, "updated_at", "is_encrypted", "updated_by_user_id") VALUES
  ('default_trial_days', '7', now(), false, NULL),
  ('registration_mode', 'open', now(), false, NULL),
  ('notify_group_link', '', now(), false, NULL),
  ('email_verification', 'false', now(), false, NULL),
  ('stripe_allowed_payment_methods', '', now(), false, NULL)
ON CONFLICT (key) DO NOTHING;

INSERT INTO plan_groups (key, name, description, "is_default", "sort_order", "created_at", "updated_at")
VALUES ('DEFAULT', '默认分组', '系统默认套餐分组', true, 10, now(), now())
ON CONFLICT (key) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_ranking_lookup
  ON playback_rankings USING btree (period, category, snapshot_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_redemptions_user_code
  ON redemptions USING btree ("user_id", code);

-- └─ 20260422_00_schema_baseline.sql ─────────────────────────────────────────────────

-- ┌─ 20260423_00_legacy_camelcase_to_snake_case.sql ─────────────────────────────────────────────────
-- 20260423_00_legacy_camelcase_to_snake_case
-- 用途：把 v1.3.1 时期仍以 camelCase 列名运行的线上库收拢到 snake_case 形态，
--       让后续 baseline 之后的增量 migration 能直接落库。
--
-- 背景：
--   - 顶层 baseline 20260422_00_schema_baseline.sql 已统一使用 snake_case
--   - 线上 v1.3.1 阶段的实际 schema 仍为 camelCase（参见 prod/prod-schema.sql）
--   - baseline 之后的所有增量 SQL 都按 snake_case 编写，例如
--     20260424_01_subscription_resubmission_after_rejection.sql 创建
--     uq_subscriptions_active_media (type, "tmdb_id", season)，未先改名会直接报错
--   - 该 migration 仅做列改名，依赖的索引由 PG 自动跟随，无需手工 drop/recreate
--
-- 改了哪些表、字段、索引、约束：
--   - 涉及表：client_blacklists, device_actions, email_verifications, media_gaps,
--     media_quality_caches, payments, plan_groups, plans, redemption_codes, redemptions,
--     settings, subscriptions, telegram_bind_codes, tmdb_cache, tv_calendar_items,
--     tv_calendar_sources, tv_calendar_subscriptions, users
--   - 仅列改名；索引名不变，索引引用的列由 PG 自动跟随新列名
--   - 不涉及任何 DROP/CREATE INDEX、不修改约束、不动 ENUM
--
-- 是否需要回填：否
-- 是否可重复执行：是
--   - 每条 RENAME 前用 information_schema.columns 双重探测（旧列存在 + 新列不存在）
--   - 已是 snake_case 的环境（含 baseline 初始化的新装库）整体 no-op
--   - 部分改名后中断的环境可重跑，从未完成的列继续
--
-- 执行约束：
--   - 该 migration 必须在 20260424_01 等后置增量之前执行
--   - lexical 顺序已保证：20260423_00_ 介于 20260422_00（baseline）和 20260424_01 之间

DO $$
DECLARE
  rec RECORD;
BEGIN
  FOR rec IN
    SELECT * FROM (VALUES
      -- (table_name, old_column, new_column)
      ('client_blacklists', 'clientName',            'client_name'),
      ('client_blacklists', 'normalizedClientName',  'normalized_client_name'),
      ('client_blacklists', 'createdAt',             'created_at'),

      ('device_actions',    'deviceId',              'device_id'),
      ('device_actions',    'userId',                'user_id'),
      ('device_actions',    'clientName',            'client_name'),
      ('device_actions',    'createdAt',             'created_at'),

      ('email_verifications','expiresAt',            'expires_at'),
      ('email_verifications','createdAt',            'created_at'),

      ('media_gaps',        'tmdbId',                'tmdb_id'),
      ('media_gaps',        'embySeriesId',          'emby_series_id'),
      ('media_gaps',        'seriesName',            'series_name'),
      ('media_gaps',        'airDate',               'air_date'),
      ('media_gaps',        'searchSnapshot',        'search_snapshot'),
      ('media_gaps',        'dispatchSnapshot',      'dispatch_snapshot'),
      ('media_gaps',        'lastScannedAt',         'last_scanned_at'),
      ('media_gaps',        'lastSearchedAt',        'last_searched_at'),
      ('media_gaps',        'requestedAt',           'requested_at'),
      ('media_gaps',        'ingestedAt',            'ingested_at'),
      ('media_gaps',        'ignoredAt',             'ignored_at'),
      ('media_gaps',        'ignoreReason',          'ignore_reason'),
      ('media_gaps',        'createdAt',             'created_at'),
      ('media_gaps',        'updatedAt',             'updated_at'),

      ('media_quality_caches','libraryId',           'library_id'),
      ('media_quality_caches','expiresAt',           'expires_at'),
      ('media_quality_caches','createdAt',           'created_at'),
      ('media_quality_caches','updatedAt',           'updated_at'),

      ('payments',          'userId',                'user_id'),
      ('payments',          'planId',                'plan_id'),
      ('payments',          'stripeSessionId',       'stripe_session_id'),
      ('payments',          'stripePaymentIntentId', 'stripe_payment_intent_id'),
      ('payments',          'createdAt',             'created_at'),
      ('payments',          'updatedAt',             'updated_at'),
      ('payments',          'checkoutUrl',           'checkout_url'),
      ('payments',          'expiresAt',             'expires_at'),

      ('plan_groups',       'isDefault',             'is_default'),
      ('plan_groups',       'sortOrder',             'sort_order'),
      ('plan_groups',       'createdAt',             'created_at'),
      ('plan_groups',       'updatedAt',             'updated_at'),

      ('plans',             'isActive',              'is_active'),
      ('plans',             'sortOrder',             'sort_order'),
      ('plans',             'createdAt',             'created_at'),
      ('plans',             'updatedAt',             'updated_at'),
      ('plans',             'planGroup',             'plan_group'),

      ('redemption_codes',  'maxUses',               'max_uses'),
      ('redemption_codes',  'usedCount',             'used_count'),
      ('redemption_codes',  'expiresAt',             'expires_at'),
      ('redemption_codes',  'defaultDays',           'default_days'),
      ('redemption_codes',  'createdAt',             'created_at'),
      ('redemption_codes',  'templateUserId',        'template_user_id'),
      ('redemption_codes',  'registrationPlanGroup', 'registration_plan_group'),

      ('redemptions',       'userId',                'user_id'),
      ('redemptions',       'createdAt',             'created_at'),

      ('settings',          'updatedAt',             'updated_at'),
      ('settings',          'isEncrypted',           'is_encrypted'),
      ('settings',          'updatedByUserId',       'updated_by_user_id'),

      ('subscriptions',     'userId',                'user_id'),
      ('subscriptions',     'tmdbId',                'tmdb_id'),
      ('subscriptions',     'posterPath',            'poster_path'),
      ('subscriptions',     'mpError',               'mp_error'),
      ('subscriptions',     'createdAt',             'created_at'),
      ('subscriptions',     'updatedAt',             'updated_at'),
      ('subscriptions',     'rejectReason',          'reject_reason'),
      ('subscriptions',     'reviewedAt',            'reviewed_at'),
      ('subscriptions',     'ingestedAt',            'ingested_at'),

      ('telegram_bind_codes','userId',               'user_id'),
      ('telegram_bind_codes','expiresAt',            'expires_at'),
      ('telegram_bind_codes','createdAt',            'created_at'),

      ('tmdb_cache',        'cacheKey',              'cache_key'),
      ('tmdb_cache',        'cacheValue',            'cache_value'),
      ('tmdb_cache',        'expiresAt',             'expires_at'),
      ('tmdb_cache',        'createdAt',             'created_at'),

      ('tv_calendar_items', 'tmdbId',                'tmdb_id'),
      ('tv_calendar_items', 'seriesId',              'series_id'),
      ('tv_calendar_items', 'airDate',               'air_date'),
      ('tv_calendar_items', 'episodeName',           'episode_name'),
      ('tv_calendar_items', 'embyItemId',            'emby_item_id'),
      ('tv_calendar_items', 'lastChecked',           'last_checked'),
      ('tv_calendar_items', 'createdAt',             'created_at'),
      ('tv_calendar_items', 'updatedAt',             'updated_at'),

      ('tv_calendar_sources','tmdbId',               'tmdb_id'),
      ('tv_calendar_sources','seriesId',             'series_id'),
      ('tv_calendar_sources','showName',             'show_name'),
      ('tv_calendar_sources','posterUrl',            'poster_url'),
      ('tv_calendar_sources','embyStatus',           'emby_status'),
      ('tv_calendar_sources','lastSyncedAt',         'last_synced_at'),
      ('tv_calendar_sources','createdAt',            'created_at'),
      ('tv_calendar_sources','updatedAt',            'updated_at'),
      ('tv_calendar_sources','lastEpisodeIngestedAt','last_episode_ingested_at'),

      ('tv_calendar_subscriptions','userId',         'user_id'),
      ('tv_calendar_subscriptions','tmdbId',         'tmdb_id'),
      ('tv_calendar_subscriptions','showName',       'show_name'),
      ('tv_calendar_subscriptions','posterUrl',      'poster_url'),
      ('tv_calendar_subscriptions','createdAt',      'created_at'),

      ('users',             'embyId',                'emby_id'),
      ('users',             'embyDisabled',          'emby_disabled'),
      ('users',             'expiresAt',             'expires_at'),
      ('users',             'isActive',              'is_active'),
      ('users',             'createdAt',             'created_at'),
      ('users',             'updatedAt',             'updated_at'),
      ('users',             'telegramId',            'telegram_id'),
      ('users',             'planGroup',             'plan_group')
    ) AS t(table_name, old_column, new_column)
  LOOP
    -- 仅当旧列存在且新列不存在时执行改名，保证幂等
    IF EXISTS (
      SELECT 1
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = rec.table_name
        AND column_name = rec.old_column
    ) AND NOT EXISTS (
      SELECT 1
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = rec.table_name
        AND column_name = rec.new_column
    ) THEN
      EXECUTE format(
        'ALTER TABLE public.%I RENAME COLUMN %I TO %I',
        rec.table_name, rec.old_column, rec.new_column
      );
    END IF;
  END LOOP;
END $$;

-- └─ 20260423_00_legacy_camelcase_to_snake_case.sql ─────────────────────────────────────────────────

-- ┌─ 20260424_01_subscription_resubmission_after_rejection.sql ─────────────────────────────────────────────────
-- Subscription resubmission after rejection
-- Changes:
-- 1) Add subscriptions.retry_from_id to link a resubmission to the rejected source record
-- 2) Drop the old global unique index uk_subscription_media
-- 3) Add a lookup index for subscriptions.retry_from_id
-- 4) Add an active-state partial unique index for type + tmdb_id + season
-- Backfill:
-- - None. Existing rows keep NULL retry_from_id.
-- Idempotency:
-- - Safe to run repeatedly via IF NOT EXISTS / IF EXISTS guards.

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS "retry_from_id" varchar(25);

DROP INDEX IF EXISTS uk_subscription_media;

CREATE INDEX IF NOT EXISTS idx_subscriptions_retry_from_id
  ON subscriptions ("retry_from_id");

CREATE UNIQUE INDEX IF NOT EXISTS uq_subscriptions_active_media
  ON subscriptions (type, "tmdb_id", season)
  WHERE status IN ('PENDING', 'APPROVED', 'INGESTED');

-- └─ 20260424_01_subscription_resubmission_after_rejection.sql ─────────────────────────────────────────────────

-- ┌─ 20260425_01_baseline_normalization_indexes.sql ─────────────────────────────────────────────────
-- Backfill baseline normalization indexes for environments upgraded from pre-baseline migrations.
--
-- baseline `20260422_00_schema_baseline.sql` still carries two historical indexes that some upgraded databases may miss
-- from the legacy production source database (the dump was made before the model
-- definitions were tightened):
--
--   - idx_ranking_lookup        on playback_rankings(period, category, snapshot_at)
--   - uq_redemptions_user_code  unique on redemptions("user_id", code)
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
  ON redemptions USING btree ("user_id", code);

-- └─ 20260425_01_baseline_normalization_indexes.sql ─────────────────────────────────────────────────

-- ┌─ 20260425_02_telegram_bind_codes_user_unique.sql ─────────────────────────────────────────────────
-- 一个用户同一时刻只允许一条 telegram_bind_codes，确保 GenerateBindCode 的 ON CONFLICT(user_id) 语义可靠。
--
-- 老库由 baseline 创建了非唯一 idx_telegram_bind_codes_user_id（按 user_id 索引），
-- 本 migration 在同一列上额外创建唯一索引，提供原子互斥保证；不删除原非唯一索引。
--
-- 历史脏数据处理：旧 GenerateBindCode 是事务里 DELETE+INSERT，在并发下可能留下同一 user_id
-- 的多条绑定码；直接 CREATE UNIQUE INDEX 会因冲突失败、API 启动期 VerifySchema 也会停摆。
-- 本 migration 在建索引前显式去重：每个 user_id 只保留 created_at 最新的一条。
-- 绑定码本就是短期凭据（5 分钟有效），删除冗余历史行不会影响业务。
--
-- 幂等：去重的 DELETE 在二次执行时 row_number() = 1 永远成立 → 0 行受影响；
--       CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等。

WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY "user_id" ORDER BY "created_at" DESC, id DESC) AS rn
    FROM telegram_bind_codes
)
DELETE FROM telegram_bind_codes
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX IF NOT EXISTS uq_telegram_bind_codes_user
  ON telegram_bind_codes USING btree ("user_id");

-- └─ 20260425_02_telegram_bind_codes_user_unique.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_01_users_lower_unique_indexes.sql ─────────────────────────────────────────────────
-- 用户名 / 邮箱大小写不敏感唯一约束。
--
-- 批次 1 把登录与注册唯一性校验改为 lower(username) / lower(email) 比较，但若数据库未补对应
-- 唯一索引，并发注册或后台创建仍会写入大小写不同的逻辑重复账号；新登录链路按 created_at ASC
-- 取最早一条，会让后注册的同名大小写账号永远登不到自己。本 migration 把唯一性约束下沉到 DB。
--
-- 历史脏数据处理：直接 CREATE UNIQUE INDEX 在已有重复时会失败，并连带 API 启动期 VerifySchema
-- 报错。本 migration 用 DO $$ ... $$ 块在建索引前显式预检；若发现冲突立即 RAISE EXCEPTION，
-- 错误信息附上排查 SQL，运维需先去重再重跑。
--
-- 幂等：CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等；预检在已建好索引的环境上为 0 行结果，DO 块直接通过。

DO $$
DECLARE
    dup_username_count int;
    dup_email_count int;
BEGIN
    SELECT count(*) INTO dup_username_count FROM (
        SELECT lower(username) FROM users GROUP BY lower(username) HAVING count(*) > 1
    ) t;

    SELECT count(*) INTO dup_email_count FROM (
        SELECT lower(email)
        FROM users
        WHERE email IS NOT NULL AND email <> ''
        GROUP BY lower(email)
        HAVING count(*) > 1
    ) t;

    IF dup_username_count > 0 OR dup_email_count > 0 THEN
        RAISE EXCEPTION
            'users 表存在大小写不敏感重复 (username=%, email=%); 排查并去重：'
            'SELECT lower(username), count(*) FROM users GROUP BY 1 HAVING count(*) > 1; '
            'SELECT lower(email), count(*) FROM users WHERE email IS NOT NULL AND email <> '''' GROUP BY 1 HAVING count(*) > 1;',
            dup_username_count,
            dup_email_count;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_username_lower
  ON users USING btree (lower(username));

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower
  ON users USING btree (lower(email))
  WHERE email IS NOT NULL AND email <> '';

-- └─ 20260426_01_users_lower_unique_indexes.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_02_failed_emby_async_ops.sql ─────────────────────────────────────────────────
-- failed_emby_async_ops：支付履约 / 兑换码 / 注册回滚等链路在事务外调 Emby 失败时
-- 的补偿队列。cron `@every 10m` 按 next_attempt_at <= now() 拉取重试，成功删除该行；
-- 失败时指数退避（30s/2m/10m/1h/6h/24h）并写 last_error，retries > 6 写 ERROR 日志告警但保留行。
--
-- 字段说明：
--   origin       业务来源：payment_unban / redemption_unban / register_cleanup
--   origin_ref_id  业务侧引用 ID：paymentId / redemptionId / emby_user_id
--   emby_user_id   待操作的 Emby 账号
--   action       unban / delete
--   payload      可选 JSON 文本（保留扩展字段，本批未使用）
--
-- 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS failed_emby_async_ops (
    id              varchar(25)  PRIMARY KEY,
    origin          varchar(32)  NOT NULL,
    "origin_ref_id"   varchar(64)  NOT NULL,
    "emby_user_id"    varchar(64)  NOT NULL,
    action          varchar(20)  NOT NULL,
    payload         text,
    retries         integer      NOT NULL DEFAULT 0,
    "next_attempt_at" timestamptz  NOT NULL DEFAULT now(),
    "last_error"     varchar(500),
    "created_at"     timestamptz  NOT NULL DEFAULT now(),
    "updated_at"     timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_failed_emby_async_ops_next
    ON failed_emby_async_ops ("next_attempt_at", retries);

CREATE INDEX IF NOT EXISTS idx_failed_emby_async_ops_origin
    ON failed_emby_async_ops (origin, "origin_ref_id");

-- └─ 20260426_02_failed_emby_async_ops.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_03_stripe_webhook_events.sql ─────────────────────────────────────────────────
-- stripe_webhook_events：Stripe webhook event.id 级别去重表。
--
-- Stripe Dashboard "Resend" / 网络抖动重投会推送同 event.id 多次；如果只靠业务侧
-- 状态机判重，跨事件类型（completed + async_payment_succeeded 同 sessionID）会无法
-- 区分；这里以 event.id 为主键，第一次 INSERT 成功才进入业务分发，后续命中 ON CONFLICT
-- 直接 200。处理完成后单事务把 status / processed_at 收口。
--
-- 字段说明：
--   event_id      Stripe event.id（evt_xxx），主键
--   event_type    Stripe event.type（checkout.session.completed 等）
--   livemode     Stripe event.livemode
--   received_at   首次收到时间
--   processed_at  业务分发完成时间
--   status       received / processed / skipped / failed
--   error_message 业务分发失败时的脱敏错误（不写完整 wrap 链）
--
-- 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS stripe_webhook_events (
    "event_id"      varchar(64)  PRIMARY KEY,
    "event_type"    varchar(64)  NOT NULL,
    livemode       boolean      NOT NULL DEFAULT false,
    "received_at"   timestamptz  NOT NULL DEFAULT now(),
    "processed_at"  timestamptz,
    status         varchar(20)  NOT NULL DEFAULT 'received',
    "error_message" varchar(500)
);

CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_received
    ON stripe_webhook_events ("received_at");

-- └─ 20260426_03_stripe_webhook_events.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_04_payments_checkout_constraints.sql ─────────────────────────────────────────────────
-- 20260426_04_payments_checkout_constraints
-- 用途：统一收口 payments 的 checkout 并发约束与 stripe_session_id 唯一索引。
--
-- 改了哪些表、字段、索引、约束：
--   1. 为 `(user_id, plan_id) WHERE status='pending'` 增加 partial unique，
--      阻断双标签页连点结账造出多条 pending 订单。
--   2. 删除 baseline 中重复的 stripe_session_id 唯一索引，保留单一 partial unique
--      `idx_payments_stripe_session_id WHERE stripe_session_id <> ''`。
--
-- 是否需要回填：
--   - 需要预检并人工收口重复 pending 订单；存在脏数据时 migration 会 fail-fast 停止。
--
-- 是否可重复执行：是。DROP INDEX IF EXISTS / CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等。

DO $$
DECLARE
    dup_count int;
BEGIN
    SELECT count(*) INTO dup_count FROM (
        SELECT "user_id", "plan_id"
        FROM payments
        WHERE status = 'pending'
        GROUP BY "user_id", "plan_id"
        HAVING count(*) > 1
    ) t;

    IF dup_count > 0 THEN
        RAISE EXCEPTION
            'payments 存在 % 组同 (user_id, plan_id) 的多条 pending 订单；先收口再重跑：'
            'SELECT "user_id", "plan_id", count(*) FROM payments WHERE status=''pending'' GROUP BY 1,2 HAVING count(*) > 1; '
            '收口示例：UPDATE payments SET status=''expired'' WHERE id IN (SELECT id FROM payments p WHERE status=''pending'' AND id NOT IN (SELECT min(id) FROM payments WHERE status=''pending'' GROUP BY "user_id","plan_id"));',
            dup_count;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_pending_user_plan
    ON payments ("user_id", "plan_id")
    WHERE status = 'pending';

DROP INDEX IF EXISTS idx_payments_stripe_session;
DROP INDEX IF EXISTS idx_payments_stripe_session_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_stripe_session_id
    ON payments ("stripe_session_id")
    WHERE "stripe_session_id" <> '';

-- └─ 20260426_04_payments_checkout_constraints.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_05_subscriptions_ingest_progress.sql ─────────────────────────────────────────────────
-- subscriptions：增加 ingest_progress 列，用于整剧（season=0）订阅的入库进度展示。
--
-- 业务背景：
--   原 `MarkSubscriptionsIngestedByWebhook` 在收到任意一集的 Emby webhook 时就把整剧
--   订阅 (season=0) 收口为 INGESTED，让用户和管理员误以为整剧入库完成；实际上还有大量
--   集数缺失。本批次拆分整剧 / 单季的命中策略：
--     - 单季订阅 (season=N)：webhook 命中即 INGEST（与原行为一致，只是查询条件收紧）
--     - 整剧订阅 (season=0)：webhook 仅更新 ingest_progress，不收口为 INGESTED
--   ingest_progress 用 "Si Ej" 记录最近一集的命中信息，便于前端 / 管理员观察整剧进度。
--   由管理员在前端通过 `MarkSubscriptionIngestedAsAdmin` 显式收口整剧入库状态。
--
-- 字段说明：
--   ingest_progress  整剧订阅的入库进度文本，最多 50 字符，可空（单季订阅永远为空）
--
-- 幂等：ADD COLUMN IF NOT EXISTS。

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS "ingest_progress" varchar(50);

-- └─ 20260426_05_subscriptions_ingest_progress.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_06_media_gaps_dispatch_failed.sql ─────────────────────────────────────────────────
-- media_gaps：增加 DISPATCH_FAILED 状态所需的 last_dispatch_error 列。
--
-- 业务背景：DispatchGap 在调用 MoviePilot 失败时原本只记录到日志，工单状态保持
-- REQUESTED / SEARCHED 不变；管理员看不到失败状态，也无法分辨"还在请求中"与
-- "请求失败但未重试"。本次新增 DISPATCH_FAILED 状态枚举（仅在应用层校验，不加 DB CHECK），
-- 并在工单上记录 last_dispatch_error，便于前端展示重试入口。
--
-- 字段说明：
--   last_dispatch_error  最近一次下发失败的脱敏错误（来自 upstream.SafeUpstreamError），最多 500 字符
--
-- 幂等：ADD COLUMN IF NOT EXISTS。

ALTER TABLE media_gaps
    ADD COLUMN IF NOT EXISTS "last_dispatch_error" varchar(500);

-- └─ 20260426_06_media_gaps_dispatch_failed.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_07_media_gap_scans.sql ─────────────────────────────────────────────────
-- media_gap_scans：缺集扫描的持久化执行记录。
--
-- 业务背景：
--   handlers/media_gap_async.go 使用进程内 sync.RWMutex 控制扫描互斥，多副本部署时
--   每个副本各自跑全库扫描，把上游 Emby / TMDB 流量放大 N 倍。本批引入 PostgreSQL
--   advisory lock + 本表，提供跨副本互斥与扫描审计记录。
--
-- 字段说明：
--   id            扫描批次 ID（cuid）
--   status        running / success / failed
--   node_id        承担本次扫描的节点标识（hostname + pid，便于运维回溯）
--   started_at     扫描开始时间
--   finished_at    扫描结束时间（success/failed 时填充）
--   error_message  失败时的脱敏错误（最多 500 字符）
--
-- 配套 cron `media-gap-scans-cleanup` (@weekly) 清理 7 天前的 success/failed 记录，
-- 避免长期堆积；running 记录不被清理，需运维手工排查（一般来源于进程 crash 未释放）。
--
-- 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS media_gap_scans (
    id             varchar(25)  PRIMARY KEY,
    status         varchar(20)  NOT NULL,
    "node_id"       varchar(64)  NOT NULL,
    "started_at"    timestamptz  NOT NULL DEFAULT now(),
    "finished_at"   timestamptz,
    "error_message" varchar(500)
);

CREATE INDEX IF NOT EXISTS idx_media_gap_scans_started
    ON media_gap_scans ("started_at");

-- └─ 20260426_07_media_gap_scans.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_08_playback_rankings_idempotency.sql ─────────────────────────────────────────────────
-- 幂等：重复执行安全
-- 1. 预清理重复 batch（保留每组最新一条）
WITH duplicates AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY period, period_start, period_end
           ORDER BY created_at DESC, id DESC
         ) AS rn
  FROM playback_rankings
)
DELETE FROM playback_rankings
WHERE id IN (SELECT id FROM duplicates WHERE rn > 1);

-- 2. 扩位 batch_id
ALTER TABLE playback_rankings
  ALTER COLUMN batch_id TYPE varchar(32);

-- 3. 添加幂等唯一索引
CREATE UNIQUE INDEX IF NOT EXISTS uq_playback_rankings_period
  ON playback_rankings (period, period_start, period_end);

-- └─ 20260426_08_playback_rankings_idempotency.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_09_media_quality_caches_inflight.sql ─────────────────────────────────────────────────
ALTER TABLE media_quality_caches
  ADD COLUMN IF NOT EXISTS "schema_version" integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS "inflight_until" timestamptz;
CREATE INDEX IF NOT EXISTS idx_media_quality_caches_inflight
  ON media_quality_caches ("inflight_until")
  WHERE "inflight_until" IS NOT NULL;

-- └─ 20260426_09_media_quality_caches_inflight.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_10_device_actions_operator_id.sql ─────────────────────────────────────────────────
ALTER TABLE device_actions
  ADD COLUMN IF NOT EXISTS "operator_id" varchar(25);
CREATE INDEX IF NOT EXISTS idx_device_actions_operator
  ON device_actions ("operator_id")
  WHERE "operator_id" IS NOT NULL;

-- └─ 20260426_10_device_actions_operator_id.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_11_tv_calendar_sources_sync_markers.sql ─────────────────────────────────────────────────
ALTER TABLE tv_calendar_sources
  ADD COLUMN IF NOT EXISTS "last_full_sync_at" timestamptz,
  ADD COLUMN IF NOT EXISTS "last_correction_at" timestamptz;

-- └─ 20260426_11_tv_calendar_sources_sync_markers.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_12_bot_pending_reject_requests.sql ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS bot_pending_reject_requests (
  id              varchar(25)  PRIMARY KEY,
  "chat_id"        bigint       NOT NULL,
  "admin_user_id"   varchar(25)  NOT NULL,
  "subscription_id" varchar(25) NOT NULL,
  "created_at"     timestamptz  NOT NULL DEFAULT now(),
  "expires_at"     timestamptz  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_bot_pending_reject_requests_chat
  ON bot_pending_reject_requests ("chat_id", "expires_at");
CREATE INDEX IF NOT EXISTS idx_bot_pending_reject_requests_expires
  ON bot_pending_reject_requests ("expires_at");

-- └─ 20260426_12_bot_pending_reject_requests.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_13_schema_alignment.sql ─────────────────────────────────────────────────
-- 20260426_13_schema_alignment
-- 用途：清理重复 / 冗余索引，将 users.telegram_id 改为 partial unique 索引，
--       清理 media_gaps 冗余 tmdb_id 单列索引。
--
-- 改了哪些表、字段、索引、约束：
--   - users: 删除 invite_code 列（如存在）及其索引；telegram_id 改 partial unique
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

-- users.invite_code 死字段（如果存在）
DROP INDEX IF EXISTS idx_users_invite_code;
ALTER TABLE users DROP COLUMN IF EXISTS "invite_code";

-- users.telegram_id 改 partial unique
-- 先删旧的全量 unique index（如存在），再建 partial unique
DROP INDEX IF EXISTS uk_users_telegram_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_telegram_id
  ON users USING btree ("telegram_id")
  WHERE "telegram_id" IS NOT NULL;

-- media_gaps tmdb_id 冗余单列索引清理
-- 复合唯一索引 uk_media_gap_episode 已覆盖 tmdb_id 的查询，单列索引冗余
DROP INDEX IF EXISTS idx_media_gaps_tmdb_id;

-- └─ 20260426_13_schema_alignment.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_14_airdate_to_date.sql ─────────────────────────────────────────────────
-- 20260426_14_airdate_to_date
-- 用途：将 tv_calendar_items.air_date 和 media_gaps.air_date 从 timestamptz 改为 date 类型，
--       避免 DST 漂移导致日期偏移一天的问题。
--
-- 改了哪些表、字段、索引、约束：
--   - tv_calendar_items.air_date: timestamptz → date
--   - media_gaps.air_date: timestamptz → date
--
-- 是否需要回填：转换时使用 AT TIME ZONE 'UTC'，仅当时间部分为 00:00:00 UTC 时才安全执行
-- 是否可重复执行：否（二次执行 ALTER COLUMN TYPE 对已是 date 的列仍会执行，但 USING 中
--                 的 AT TIME ZONE 对 date 类型无效；生产执行前请先用预检查 SQL 验证）
--
-- 执行前预检（两条都返回 0 才能安全执行）：
-- SELECT count(*) FROM tv_calendar_items WHERE EXTRACT(HOUR FROM "air_date" AT TIME ZONE 'UTC') != 0;
-- SELECT count(*) FROM media_gaps WHERE EXTRACT(HOUR FROM "air_date" AT TIME ZONE 'UTC') != 0;

ALTER TABLE tv_calendar_items
  ALTER COLUMN "air_date" TYPE date USING ("air_date" AT TIME ZONE 'UTC')::date;

ALTER TABLE media_gaps
  ALTER COLUMN "air_date" TYPE date USING ("air_date" AT TIME ZONE 'UTC')::date;

-- └─ 20260426_14_airdate_to_date.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_15_users_password_reset_required.sql ─────────────────────────────────────────────────
-- 20260426_15_users_password_reset_required
-- 用途：为 users 表新增 password_reset_required 列，用于标记首次登录必须修改密码。
--
-- 改了哪些表、字段、索引、约束：
--   - users 表新增 password_reset_required boolean 列（NOT NULL DEFAULT false）
--
-- 是否需要回填：否（DEFAULT false 对所有已有用户生效）
-- 是否可重复执行：是（ADD COLUMN IF NOT EXISTS）

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS "password_reset_required" boolean NOT NULL DEFAULT false;

-- └─ 20260426_15_users_password_reset_required.sql ─────────────────────────────────────────────────

-- ┌─ 20260426_16_subscriptions_note_not_null.sql ─────────────────────────────────────────────────
-- 20260426_16_subscriptions_note_not_null
-- 用途：将 subscriptions.note 列从可空改为 NOT NULL DEFAULT ''，
--       统一空备注语义，消灭 NULL 判断路径。
--
-- 改了哪些表、字段、索引、约束：
--   - subscriptions.note: NULL → NOT NULL DEFAULT ''
--
-- 是否需要回填：是，先将 NULL 回填为空字符串
-- 是否可重复执行：是（SET NOT NULL 对已是 NOT NULL 的列是 no-op；UPDATE 对无 NULL 的表是 no-op）

UPDATE subscriptions SET note = '' WHERE note IS NULL;
ALTER TABLE subscriptions ALTER COLUMN note SET NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN note SET DEFAULT '';

-- └─ 20260426_16_subscriptions_note_not_null.sql ─────────────────────────────────────────────────

-- ┌─ 20260427_01_bot_runtime_locks.sql ─────────────────────────────────────────────────
-- bot_runtime_locks：Bot polling 模式单实例租约锁。
--
-- 用途：
--   - Bot 在 TELEGRAM_UPDATE_MODE=polling 时通过 Internal API 申请/续租/释放租约
--   - 多实例竞争时只有一份 owner_id 能持有 telegram_polling 锁
--   - owner 崩溃不释放时，租约到期后允许下一实例接管
--
-- 幂等：CREATE TABLE/INDEX IF NOT EXISTS

CREATE TABLE IF NOT EXISTS bot_runtime_locks (
  name        varchar(100) PRIMARY KEY,
  "owner_id"   varchar(200) NOT NULL,
  "expires_at" timestamptz  NOT NULL,
  "created_at" timestamptz  NOT NULL DEFAULT now(),
  "updated_at" timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bot_runtime_locks_expires
  ON bot_runtime_locks ("expires_at");

-- └─ 20260427_01_bot_runtime_locks.sql ─────────────────────────────────────────────────

-- ┌─ 20260427_02_media_gaps_ignore_reason_code.sql ─────────────────────────────────────────────────
-- 20260427_02_media_gaps_ignore_reason_code
-- 用途：为 media_gaps 增加 ignore_reason_code，显式区分人工忽略与系统忽略。
-- 变更：
--   - media_gaps 新增 ignore_reason_code varchar(50) 可空列
--   - 回填当前由系统扫描收口产生的 "season not activated in library" 为 season_not_activated
--   - 其余历史 IGNORED 行保持空值，避免仅凭文本误判来源
-- 幂等：是，可重复执行。

ALTER TABLE media_gaps
  ADD COLUMN IF NOT EXISTS "ignore_reason_code" varchar(50);

UPDATE media_gaps
SET "ignore_reason_code" = 'season_not_activated'
WHERE status = 'IGNORED'
  AND "ignore_reason_code" IS NULL
  AND "ignore_reason" = 'season not activated in library';

-- └─ 20260427_02_media_gaps_ignore_reason_code.sql ─────────────────────────────────────────────────

-- ┌─ 20260427_04_bot_pending_reject_message_context.sql ─────────────────────────────────────────────────
-- 20260427_04_bot_pending_reject_message_context
-- 用途：为 bot_pending_reject_requests 增加拒绝订阅消息上下文，避免 Bot 重启或滚动发布导致待输入状态丢失。
-- 变更：
--   - 新增 message_id / has_photo / original_text 三列
-- 幂等：是，可重复执行。

ALTER TABLE bot_pending_reject_requests
  ADD COLUMN IF NOT EXISTS "message_id" bigint,
  ADD COLUMN IF NOT EXISTS "has_photo" boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS "original_text" text NOT NULL DEFAULT '';

-- └─ 20260427_04_bot_pending_reject_message_context.sql ─────────────────────────────────────────────────
