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
    id character varying(25) NOT NULL,
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
-- Name: plan_groups plan_groups_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY plan_groups
    ADD CONSTRAINT plan_groups_key_key UNIQUE (key);


--
-- Name: plan_groups plan_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY plan_groups
    ADD CONSTRAINT plan_groups_pkey PRIMARY KEY (id);


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
