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
-- Name: MediaType; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE "MediaType" AS ENUM (
    'MOVIE',
    'TV'
);


--
-- Name: SubscriptionStatus; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE "SubscriptionStatus" AS ENUM (
    'PENDING',
    'APPROVED',
    'REJECTED'
);


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: client_blacklists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE client_blacklists (
    id character varying(25) NOT NULL,
    "clientName" character varying(100) NOT NULL,
    "normalizedClientName" character varying(100) NOT NULL,
    reason character varying(255) DEFAULT ''::character varying,
    "createdAt" timestamp with time zone DEFAULT now()
);


--
-- Name: device_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE device_actions (
    id character varying(25) NOT NULL,
    "deviceId" character varying(100) DEFAULT ''::character varying,
    "userId" character varying(25) DEFAULT ''::character varying,
    "clientName" character varying(100) DEFAULT ''::character varying,
    action character varying(50) NOT NULL,
    note character varying(255) DEFAULT ''::character varying,
    "createdAt" timestamp with time zone DEFAULT now()
);


--
-- Name: email_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE email_verifications (
    id character varying(25) NOT NULL,
    email character varying(255) NOT NULL,
    code character varying(6) NOT NULL,
    ip character varying(45) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now(),
    type character varying(20) DEFAULT 'register'::character varying NOT NULL
);


--
-- Name: media_quality_caches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE media_quality_caches (
    id character varying(25) NOT NULL,
    "libraryId" character varying(100) NOT NULL,
    statistics text NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now(),
    "updatedAt" timestamp with time zone DEFAULT now()
);


--
-- Name: payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payments (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    "planId" character varying(25) NOT NULL,
    "stripeSessionId" character varying(255) NOT NULL,
    "stripePaymentIntentId" character varying(255) DEFAULT ''::character varying,
    amount bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    days bigint NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now(),
    "updatedAt" timestamp with time zone DEFAULT now(),
    "checkoutUrl" character varying(2048) DEFAULT ''::character varying NOT NULL,
    "expiresAt" timestamp with time zone
);


--
-- Name: plan_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE plan_groups (
    key character varying(50) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    "isDefault" boolean DEFAULT false NOT NULL,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE plans (
    id character varying(25) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying,
    days bigint NOT NULL,
    price bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    "isActive" boolean DEFAULT true NOT NULL,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now(),
    "updatedAt" timestamp with time zone DEFAULT now(),
    "planGroup" character varying(50) DEFAULT 'DEFAULT'::character varying NOT NULL
);


--
-- Name: playback_rankings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE playback_rankings (
    id character varying(25) NOT NULL,
    period character varying(10) NOT NULL,
    category character varying(20) NOT NULL,
    rank bigint NOT NULL,
    item_name character varying(500) NOT NULL,
    play_count bigint NOT NULL,
    duration bigint NOT NULL,
    snapshot_at timestamp with time zone NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    batch_id character varying(25) DEFAULT ''::character varying NOT NULL,
    item_key character varying(128) DEFAULT ''::character varying NOT NULL,
    item_source_type character varying(32) DEFAULT ''::character varying NOT NULL
);


--
-- Name: redemption_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE redemption_codes (
    id character varying(25) NOT NULL,
    code character varying(20) NOT NULL,
    "maxUses" bigint DEFAULT 1 NOT NULL,
    "usedCount" bigint DEFAULT 0 NOT NULL,
    "expiresAt" timestamp with time zone,
    "defaultDays" bigint DEFAULT 30 NOT NULL,
    "createdAt" timestamp with time zone,
    "templateUserId" character varying(25),
    notes character varying(500) DEFAULT ''::character varying NOT NULL,
    "registrationPlanGroup" character varying(50)
);


--
-- Name: redemptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE redemptions (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    code character varying(20) NOT NULL,
    days bigint NOT NULL,
    "createdAt" timestamp with time zone
);


--
-- Name: settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE settings (
    key character varying(100) NOT NULL,
    value text NOT NULL,
    "updatedAt" timestamp with time zone,
    "isEncrypted" boolean DEFAULT false NOT NULL,
    "updatedByUserId" character varying(25)
);


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE subscriptions (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    type character varying(10) NOT NULL,
    name character varying(255) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "posterPath" character varying(500),
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    note text,
    "mpError" character varying(500),
    "createdAt" timestamp with time zone,
    "updatedAt" timestamp with time zone,
    season integer DEFAULT 0 NOT NULL
);


--
-- Name: telegram_bind_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE telegram_bind_codes (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    code character varying(6) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now()
);


--
-- Name: tmdb_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tmdb_cache (
    id character varying(25) NOT NULL,
    "cacheKey" character varying(255) NOT NULL,
    "cacheValue" text NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now()
);


--
-- Name: tv_calendar_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tv_calendar_items (
    id character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "seriesId" character varying(50) DEFAULT ''::character varying,
    season bigint NOT NULL,
    episode bigint NOT NULL,
    "airDate" timestamp with time zone NOT NULL,
    "episodeName" character varying(255) DEFAULT ''::character varying,
    status character varying(20) DEFAULT 'upcoming'::character varying NOT NULL,
    "embyItemId" character varying(50) DEFAULT ''::character varying,
    "lastChecked" timestamp with time zone DEFAULT now(),
    "createdAt" timestamp with time zone DEFAULT now(),
    "updatedAt" timestamp with time zone DEFAULT now(),
    overview text DEFAULT ''::text NOT NULL
);


--
-- Name: tv_calendar_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tv_calendar_sources (
    id character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "seriesId" character varying(50) DEFAULT ''::character varying NOT NULL,
    "showName" character varying(255) DEFAULT ''::character varying NOT NULL,
    "posterUrl" character varying(500) DEFAULT ''::character varying NOT NULL,
    overview text DEFAULT ''::text NOT NULL,
    "embyStatus" character varying(20) DEFAULT 'continuing'::character varying NOT NULL,
    "lastSyncedAt" timestamp with time zone,
    "createdAt" timestamp with time zone DEFAULT now(),
    "updatedAt" timestamp with time zone DEFAULT now(),
    "lastEpisodeIngestedAt" timestamp with time zone
);


--
-- Name: tv_calendar_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tv_calendar_subscriptions (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "showName" character varying(255) NOT NULL,
    "posterUrl" character varying(500) DEFAULT ''::character varying,
    "createdAt" timestamp with time zone DEFAULT now()
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE users (
    id character varying(25) NOT NULL,
    username character varying(50) NOT NULL,
    role character varying(10) DEFAULT 'user'::character varying NOT NULL,
    password text,
    email character varying(255),
    "embyId" character varying(50),
    "inviteCode" character varying(20),
    "expiresAt" timestamp with time zone,
    "isActive" boolean DEFAULT true NOT NULL,
    "createdAt" timestamp with time zone,
    "updatedAt" timestamp with time zone,
    "embyDisabled" boolean DEFAULT false NOT NULL,
    "telegramId" bigint,
    "planGroup" character varying(50)
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
-- Name: idx_client_blacklists_normalized_client_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_client_blacklists_normalized_client_name ON client_blacklists USING btree ("normalizedClientName");


--
-- Name: idx_device_actions_device_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_actions_device_id ON device_actions USING btree ("deviceId");


--
-- Name: idx_device_actions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_actions_user_id ON device_actions USING btree ("userId");


--
-- Name: idx_email_verifications_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_email ON email_verifications USING btree (email);


--
-- Name: idx_email_verifications_ip; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_ip ON email_verifications USING btree (ip);


--
-- Name: idx_email_verifications_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_type ON email_verifications USING btree (type);


--
-- Name: idx_media_quality_caches_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_quality_caches_expires_at ON media_quality_caches USING btree ("expiresAt");


--
-- Name: idx_media_quality_caches_library_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_media_quality_caches_library_id ON media_quality_caches USING btree ("libraryId");


--
-- Name: idx_payments_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_expires_at ON payments USING btree ("expiresAt");


--
-- Name: idx_payments_plan_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_plan_id ON payments USING btree ("planId");


--
-- Name: idx_payments_stripe_session; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payments_stripe_session ON payments USING btree ("stripeSessionId");


--
-- Name: idx_payments_stripe_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payments_stripe_session_id ON payments USING btree ("stripeSessionId");


--
-- Name: idx_payments_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_user_id ON payments USING btree ("userId");


--
-- Name: idx_plan_groups_is_default; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plan_groups_is_default ON plan_groups USING btree ("isDefault");


--
-- Name: idx_plan_groups_sort_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plan_groups_sort_order ON plan_groups USING btree ("sortOrder");


--
-- Name: idx_plans_active_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_active_sort ON plans USING btree ("isActive", "sortOrder");


--
-- Name: idx_plans_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_plan_group ON plans USING btree ("planGroup");


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

CREATE INDEX idx_redemption_codes_registration_plan_group ON redemption_codes USING btree ("registrationPlanGroup");


--
-- Name: idx_redemption_codes_template_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemption_codes_template_user_id ON redemption_codes USING btree ("templateUserId");


--
-- Name: idx_redemptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemptions_user_id ON redemptions USING btree ("userId");


--
-- Name: idx_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_user_id ON subscriptions USING btree ("userId");


--
-- Name: idx_telegram_bind_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_telegram_bind_codes_code ON telegram_bind_codes USING btree (code);


--
-- Name: idx_telegram_bind_codes_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_bind_codes_user_id ON telegram_bind_codes USING btree ("userId");


--
-- Name: idx_tmdb_cache_cache_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_tmdb_cache_cache_key ON tmdb_cache USING btree ("cacheKey");


--
-- Name: idx_tmdb_cache_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tmdb_cache_expires_at ON tmdb_cache USING btree ("expiresAt");


--
-- Name: idx_tv_calendar_items_air_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_air_date ON tv_calendar_items USING btree ("airDate");


--
-- Name: idx_tv_calendar_items_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_series_id ON tv_calendar_items USING btree ("seriesId");


--
-- Name: idx_tv_calendar_items_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_tmdb_id ON tv_calendar_items USING btree ("tmdbId");


--
-- Name: idx_tv_calendar_sources_last_episode_ingested_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_sources_last_episode_ingested_at ON tv_calendar_sources USING btree ("lastEpisodeIngestedAt");


--
-- Name: idx_tv_calendar_sources_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_sources_series_id ON tv_calendar_sources USING btree ("seriesId");


--
-- Name: idx_tv_calendar_sources_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_tv_calendar_sources_tmdb_id ON tv_calendar_sources USING btree ("tmdbId");


--
-- Name: idx_tv_calendar_subscriptions_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_subscriptions_tmdb_id ON tv_calendar_subscriptions USING btree ("tmdbId");


--
-- Name: idx_tv_calendar_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_subscriptions_user_id ON tv_calendar_subscriptions USING btree ("userId");


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_email ON users USING btree (email);


--
-- Name: idx_users_emby_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_emby_id ON users USING btree ("embyId");


--
-- Name: idx_users_invite_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_invite_code ON users USING btree ("inviteCode");


--
-- Name: idx_users_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_plan_group ON users USING btree ("planGroup");


--
-- Name: idx_users_telegram_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_telegram_id ON users USING btree ("telegramId") WHERE ("telegramId" IS NOT NULL);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_username ON users USING btree (username);


--
-- Name: uk_subscription_media; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_subscription_media ON subscriptions USING btree (type, "tmdbId", season);


--
-- Name: uk_tv_calendar_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_tv_calendar_episode ON tv_calendar_items USING btree ("tmdbId", season, episode);


--
-- Name: uk_tv_calendar_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_tv_calendar_subscription ON tv_calendar_subscriptions USING btree ("userId", "tmdbId");


--
-- Name: uq_client_blacklists_normalized_client_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_client_blacklists_normalized_client_name ON client_blacklists USING btree ("normalizedClientName");


--
-- Name: uq_media_quality_caches_library_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_media_quality_caches_library_id ON media_quality_caches USING btree ("libraryId");


--
-- Name: uq_plan_groups_default_true; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_plan_groups_default_true ON plan_groups USING btree ("isDefault") WHERE ("isDefault" = true);


--
-- Name: uq_tmdb_cache_cache_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tmdb_cache_cache_key ON tmdb_cache USING btree ("cacheKey");


--
-- Name: uq_tv_calendar_items_tmdb_season_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_items_tmdb_season_episode ON tv_calendar_items USING btree ("tmdbId", season, episode);


--
-- Name: uq_tv_calendar_sources_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_sources_tmdb_id ON tv_calendar_sources USING btree ("tmdbId");


--
-- Name: uq_tv_calendar_subscriptions_user_tmdb; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_subscriptions_user_tmdb ON tv_calendar_subscriptions USING btree ("userId", "tmdbId");


--
-- PostgreSQL database dump complete
--



-- Ember baseline normalization: deterministic bootstrap data and missing migration indexes.

INSERT INTO settings (key, value, "updatedAt", "isEncrypted", "updatedByUserId") VALUES
  ('default_trial_days', '7', now(), false, NULL),
  ('registration_mode', 'open', now(), false, NULL),
  ('notify_group_link', '', now(), false, NULL),
  ('email_verification', 'false', now(), false, NULL),
  ('stripe_allowed_payment_methods', '', now(), false, NULL)
ON CONFLICT (key) DO NOTHING;

INSERT INTO plan_groups (key, name, description, "isDefault", "sortOrder", "createdAt", "updatedAt")
VALUES ('DEFAULT', '默认分组', '系统默认套餐分组', true, 10, now(), now())
ON CONFLICT (key) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_ranking_lookup
  ON playback_rankings USING btree (period, category, snapshot_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_redemptions_user_code
  ON redemptions USING btree ("userId", code);
