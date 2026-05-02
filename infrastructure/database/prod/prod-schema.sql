--
-- PostgreSQL database dump
--

\restrict hyfWfZuUyfE7BsDNOCL4w8HB9i4mRk2on1wMDykn7369slRTevAvj2qhKjtjtUW

-- Dumped from database version 15.15
-- Dumped by pg_dump version 15.15

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: MediaType; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public."MediaType" AS ENUM (
    'MOVIE',
    'TV'
);


--
-- Name: SubscriptionStatus; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public."SubscriptionStatus" AS ENUM (
    'PENDING',
    'APPROVED',
    'REJECTED'
);


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: client_blacklists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.client_blacklists (
    id character varying(25) NOT NULL,
    "clientName" character varying(100) NOT NULL,
    "normalizedClientName" character varying(100) NOT NULL,
    reason character varying(255) DEFAULT ''::character varying NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: device_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_actions (
    id character varying(25) NOT NULL,
    "deviceId" character varying(100) DEFAULT ''::character varying NOT NULL,
    "userId" character varying(25) DEFAULT ''::character varying NOT NULL,
    "clientName" character varying(100) DEFAULT ''::character varying NOT NULL,
    action character varying(50) NOT NULL,
    note character varying(255) DEFAULT ''::character varying NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: email_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_verifications (
    id character varying(25) NOT NULL,
    email character varying(255) NOT NULL,
    code character varying(6) NOT NULL,
    ip character varying(45) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    type character varying(20) DEFAULT 'register'::character varying NOT NULL
);


--
-- Name: media_gaps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.media_gaps (
    id character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "embySeriesId" character varying(50) DEFAULT ''::character varying NOT NULL,
    "seriesName" character varying(255) DEFAULT ''::character varying NOT NULL,
    season integer NOT NULL,
    episode integer NOT NULL,
    "airDate" timestamp with time zone NOT NULL,
    status character varying(20) DEFAULT 'MISSING'::character varying NOT NULL,
    "searchSnapshot" text DEFAULT ''::text NOT NULL,
    "dispatchSnapshot" text DEFAULT ''::text NOT NULL,
    "lastScannedAt" timestamp with time zone,
    "lastSearchedAt" timestamp with time zone,
    "requestedAt" timestamp with time zone,
    "ingestedAt" timestamp with time zone,
    "ignoredAt" timestamp with time zone,
    "ignoreReason" text DEFAULT ''::text NOT NULL,
    "createdAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: media_quality_caches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.media_quality_caches (
    id character varying(25) NOT NULL,
    "libraryId" character varying(100) NOT NULL,
    statistics text NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payments (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    "planId" character varying(25) NOT NULL,
    "stripeSessionId" character varying(255) NOT NULL,
    "stripePaymentIntentId" character varying(255) DEFAULT ''::character varying NOT NULL,
    amount bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    days integer NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT now() NOT NULL,
    "checkoutUrl" character varying(2048) DEFAULT ''::character varying NOT NULL,
    "expiresAt" timestamp with time zone
);


--
-- Name: plan_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plan_groups (
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

CREATE TABLE public.plans (
    id character varying(25) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    days integer NOT NULL,
    price bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    "isActive" boolean DEFAULT true NOT NULL,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT now() NOT NULL,
    "planGroup" character varying(50) DEFAULT 'DEFAULT'::character varying NOT NULL
);


--
-- Name: playback_rankings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.playback_rankings (
    id character varying(25) NOT NULL,
    period character varying(10) NOT NULL,
    category character varying(20) NOT NULL,
    rank integer NOT NULL,
    item_name character varying(500) NOT NULL,
    play_count integer NOT NULL,
    duration bigint NOT NULL,
    snapshot_at timestamp with time zone NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    batch_id character varying(25) DEFAULT ''::character varying NOT NULL,
    item_key character varying(128) DEFAULT ''::character varying NOT NULL,
    item_source_type character varying(32) DEFAULT ''::character varying NOT NULL
);


--
-- Name: redemption_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.redemption_codes (
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

CREATE TABLE public.redemptions (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    code character varying(20) NOT NULL,
    days bigint NOT NULL,
    "createdAt" timestamp with time zone
);


--
-- Name: settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.settings (
    key character varying(100) NOT NULL,
    value text NOT NULL,
    "updatedAt" timestamp with time zone,
    "isEncrypted" boolean DEFAULT false NOT NULL,
    "updatedByUserId" character varying(25)
);


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscriptions (
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
    season integer DEFAULT 0 NOT NULL,
    "rejectReason" text,
    "reviewedAt" timestamp with time zone,
    "ingestedAt" timestamp with time zone
);


--
-- Name: telegram_bind_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telegram_bind_codes (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    code character varying(6) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tmdb_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tmdb_cache (
    id character varying(25) NOT NULL,
    "cacheKey" character varying(255) NOT NULL,
    "cacheValue" text NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tv_calendar_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tv_calendar_items (
    id character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "seriesId" character varying(50) DEFAULT ''::character varying NOT NULL,
    season integer NOT NULL,
    episode integer NOT NULL,
    "airDate" timestamp with time zone NOT NULL,
    "episodeName" character varying(255) DEFAULT ''::character varying NOT NULL,
    overview text DEFAULT ''::text NOT NULL,
    status character varying(20) DEFAULT 'upcoming'::character varying NOT NULL,
    "embyItemId" character varying(50) DEFAULT ''::character varying NOT NULL,
    "lastChecked" timestamp with time zone DEFAULT now() NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tv_calendar_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tv_calendar_sources (
    id character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "seriesId" character varying(50) DEFAULT ''::character varying NOT NULL,
    "showName" character varying(255) DEFAULT ''::character varying NOT NULL,
    "posterUrl" character varying(500) DEFAULT ''::character varying NOT NULL,
    overview text DEFAULT ''::text NOT NULL,
    "embyStatus" character varying(20) DEFAULT 'continuing'::character varying NOT NULL,
    "lastSyncedAt" timestamp with time zone,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT now() NOT NULL,
    "lastEpisodeIngestedAt" timestamp with time zone
);


--
-- Name: tv_calendar_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tv_calendar_subscriptions (
    id character varying(25) NOT NULL,
    "userId" character varying(25) NOT NULL,
    "tmdbId" character varying(50) NOT NULL,
    "showName" character varying(255) NOT NULL,
    "posterUrl" character varying(500) DEFAULT ''::character varying NOT NULL,
    "createdAt" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id character varying(25) NOT NULL,
    username character varying(50) NOT NULL,
    role character varying(10) DEFAULT 'user'::character varying NOT NULL,
    password text,
    email character varying(255),
    "embyId" character varying(50),
    "embyDisabled" boolean DEFAULT false NOT NULL,
    "expiresAt" timestamp with time zone,
    "isActive" boolean DEFAULT true NOT NULL,
    "createdAt" timestamp with time zone,
    "updatedAt" timestamp with time zone,
    "telegramId" bigint,
    "planGroup" character varying(50)
);


--
-- Name: client_blacklists client_blacklists_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.client_blacklists
    ADD CONSTRAINT client_blacklists_pkey PRIMARY KEY (id);


--
-- Name: device_actions device_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_actions
    ADD CONSTRAINT device_actions_pkey PRIMARY KEY (id);


--
-- Name: email_verifications email_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);


--
-- Name: media_gaps media_gaps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_gaps
    ADD CONSTRAINT media_gaps_pkey PRIMARY KEY (id);


--
-- Name: media_quality_caches media_quality_caches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_quality_caches
    ADD CONSTRAINT media_quality_caches_pkey PRIMARY KEY (id);


--
-- Name: payments payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (id);


--
-- Name: plan_groups plan_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_groups
    ADD CONSTRAINT plan_groups_pkey PRIMARY KEY (key);


--
-- Name: plans plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);


--
-- Name: playback_rankings playback_rankings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.playback_rankings
    ADD CONSTRAINT playback_rankings_pkey PRIMARY KEY (id);


--
-- Name: redemption_codes redemption_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.redemption_codes
    ADD CONSTRAINT redemption_codes_pkey PRIMARY KEY (id);


--
-- Name: redemptions redemptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.redemptions
    ADD CONSTRAINT redemptions_pkey PRIMARY KEY (id);


--
-- Name: settings settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: telegram_bind_codes telegram_bind_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_bind_codes
    ADD CONSTRAINT telegram_bind_codes_pkey PRIMARY KEY (id);


--
-- Name: tmdb_cache tmdb_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tmdb_cache
    ADD CONSTRAINT tmdb_cache_pkey PRIMARY KEY (id);


--
-- Name: tv_calendar_items tv_calendar_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tv_calendar_items
    ADD CONSTRAINT tv_calendar_items_pkey PRIMARY KEY (id);


--
-- Name: tv_calendar_sources tv_calendar_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tv_calendar_sources
    ADD CONSTRAINT tv_calendar_sources_pkey PRIMARY KEY (id);


--
-- Name: tv_calendar_subscriptions tv_calendar_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tv_calendar_subscriptions
    ADD CONSTRAINT tv_calendar_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: idx_device_actions_device_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_actions_device_id ON public.device_actions USING btree ("deviceId");


--
-- Name: idx_device_actions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_actions_user_id ON public.device_actions USING btree ("userId");


--
-- Name: idx_email_verifications_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_email ON public.email_verifications USING btree (email);


--
-- Name: idx_email_verifications_ip; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_ip ON public.email_verifications USING btree (ip);


--
-- Name: idx_email_verifications_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_type ON public.email_verifications USING btree (type);


--
-- Name: idx_media_gaps_emby_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_gaps_emby_series_id ON public.media_gaps USING btree ("embySeriesId");


--
-- Name: idx_media_gaps_status_air_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_gaps_status_air_date ON public.media_gaps USING btree (status, "airDate");


--
-- Name: idx_media_gaps_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_gaps_tmdb_id ON public.media_gaps USING btree ("tmdbId");


--
-- Name: idx_media_quality_caches_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_quality_caches_expires_at ON public.media_quality_caches USING btree ("expiresAt");


--
-- Name: idx_payments_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_expires_at ON public.payments USING btree ("expiresAt");


--
-- Name: idx_payments_plan_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_plan_id ON public.payments USING btree ("planId");


--
-- Name: idx_payments_stripe_session; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payments_stripe_session ON public.payments USING btree ("stripeSessionId");


--
-- Name: idx_payments_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_user_id ON public.payments USING btree ("userId");


--
-- Name: idx_plan_groups_is_default; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plan_groups_is_default ON public.plan_groups USING btree ("isDefault");


--
-- Name: idx_plan_groups_sort_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plan_groups_sort_order ON public.plan_groups USING btree ("sortOrder");


--
-- Name: idx_plans_active_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_active_sort ON public.plans USING btree ("isActive", "sortOrder");


--
-- Name: idx_plans_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_plan_group ON public.plans USING btree ("planGroup");


--
-- Name: idx_ranking_batch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ranking_batch ON public.playback_rankings USING btree (batch_id, category, rank);


--
-- Name: idx_ranking_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ranking_item ON public.playback_rankings USING btree (period, category, item_key, period_start, period_end);


--
-- Name: idx_ranking_period_window; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ranking_period_window ON public.playback_rankings USING btree (period, period_start, period_end, snapshot_at);


--
-- Name: idx_redemption_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_redemption_codes_code ON public.redemption_codes USING btree (code);


--
-- Name: idx_redemption_codes_registration_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemption_codes_registration_plan_group ON public.redemption_codes USING btree ("registrationPlanGroup");


--
-- Name: idx_redemption_codes_template_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemption_codes_template_user_id ON public.redemption_codes USING btree ("templateUserId");


--
-- Name: idx_redemptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redemptions_user_id ON public.redemptions USING btree ("userId");


--
-- Name: idx_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_user_id ON public.subscriptions USING btree ("userId");


--
-- Name: idx_telegram_bind_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_telegram_bind_codes_code ON public.telegram_bind_codes USING btree (code);


--
-- Name: idx_telegram_bind_codes_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_bind_codes_user_id ON public.telegram_bind_codes USING btree ("userId");


--
-- Name: idx_tmdb_cache_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tmdb_cache_expires_at ON public.tmdb_cache USING btree ("expiresAt");


--
-- Name: idx_tv_calendar_items_air_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_air_date ON public.tv_calendar_items USING btree ("airDate");


--
-- Name: idx_tv_calendar_items_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_series_id ON public.tv_calendar_items USING btree ("seriesId");


--
-- Name: idx_tv_calendar_items_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_items_tmdb_id ON public.tv_calendar_items USING btree ("tmdbId");


--
-- Name: idx_tv_calendar_sources_last_episode_ingested_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_sources_last_episode_ingested_at ON public.tv_calendar_sources USING btree ("lastEpisodeIngestedAt");


--
-- Name: idx_tv_calendar_sources_series_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_sources_series_id ON public.tv_calendar_sources USING btree ("seriesId");


--
-- Name: idx_tv_calendar_subscriptions_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_subscriptions_tmdb_id ON public.tv_calendar_subscriptions USING btree ("tmdbId");


--
-- Name: idx_tv_calendar_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tv_calendar_subscriptions_user_id ON public.tv_calendar_subscriptions USING btree ("userId");


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_emby_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_emby_id ON public.users USING btree ("embyId");


--
-- Name: idx_users_plan_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_plan_group ON public.users USING btree ("planGroup");


--
-- Name: idx_users_telegram_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_telegram_id ON public.users USING btree ("telegramId") WHERE ("telegramId" IS NOT NULL);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_username ON public.users USING btree (username);


--
-- Name: uk_media_gap_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_media_gap_episode ON public.media_gaps USING btree ("tmdbId", season, episode);


--
-- Name: uk_subscription_media; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uk_subscription_media ON public.subscriptions USING btree (type, "tmdbId", season);


--
-- Name: uq_client_blacklists_normalized_client_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_client_blacklists_normalized_client_name ON public.client_blacklists USING btree ("normalizedClientName");


--
-- Name: uq_media_quality_caches_library_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_media_quality_caches_library_id ON public.media_quality_caches USING btree ("libraryId");


--
-- Name: uq_plan_groups_default_true; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_plan_groups_default_true ON public.plan_groups USING btree ("isDefault") WHERE ("isDefault" = true);


--
-- Name: uq_redemptions_user_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_redemptions_user_code ON public.redemptions USING btree ("userId", code);


--
-- Name: uq_tmdb_cache_cache_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tmdb_cache_cache_key ON public.tmdb_cache USING btree ("cacheKey");


--
-- Name: uq_tv_calendar_items_tmdb_season_episode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_items_tmdb_season_episode ON public.tv_calendar_items USING btree ("tmdbId", season, episode);


--
-- Name: uq_tv_calendar_sources_tmdb_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_sources_tmdb_id ON public.tv_calendar_sources USING btree ("tmdbId");


--
-- Name: uq_tv_calendar_subscriptions_user_tmdb; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tv_calendar_subscriptions_user_tmdb ON public.tv_calendar_subscriptions USING btree ("userId", "tmdbId");


--
-- PostgreSQL database dump complete
--

\unrestrict hyfWfZuUyfE7BsDNOCL4w8HB9i4mRk2on1wMDykn7369slRTevAvj2qhKjtjtUW

