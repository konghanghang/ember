-- 00000000_baseline_20260605.sql
--
-- v1.6.0 截点 fresh-install schema baseline（cutoff: 2026-06-05）
--
-- 本文件是 v1.6.0 上线后唯一的"新装库初始化入口"。
-- 内容基于 2026-05-02 fresh-install baseline 吸收 2026-05-04 至 2026-05-29
-- 顶层 forward-only migration 整理而来：
--   - 去除 pg_dump 通用 SET preamble 与 \restrict/\unrestrict 修饰符
--   - 去除 public. schema 前缀以与历史 baseline 风格一致
--   - 表 / 索引按字典序排列，便于后续 schema diff 验证
--   - 末尾追加 deterministic seed（settings 6 条 + plan_groups.DEFAULT + 默认 Emby Policy 模板）
--
-- 执行行为：
--   - 仅在新空库分支被真实执行（业务核心表不存在 + schema_migrations 为空）
--   - 老库重启时由 baseline rename 豁免逻辑识别，仅写记账行不重跑 SQL
--   - 同目录里 baseline 任何时刻必须只有一份；多份共存启动期 fail-fast
--
-- 真相源优先级：
--   1) 本文件直接对齐 2026-06-05 截点 schema
--   2) 字段语义对齐 services/api/internal/models/ 下 GORM 模型
--   3) services/api/internal/db/db.go 的 schemaFingerprintColumns/Indexes 必须全部命中
--
-- 2026-05-02 baseline 与其后的顶层增量整批归档于
-- archive/pre-20260605/，仅供追溯，不再属于现行执行链路。
--
-- 形态切换决策与背景见 docs/archive/plan/architecture/baseline-fresh-install-rewrite.md。


-- ┌─ 表定义 ───────────────────────────────────────────────────────────

CREATE TABLE bot_pending_reject_requests (
    id character varying(25) NOT NULL,
    chat_id bigint NOT NULL,
    admin_user_id character varying(25) NOT NULL,
    subscription_id character varying(25) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    message_id bigint,
    has_photo boolean DEFAULT false NOT NULL,
    original_text text DEFAULT ''::text NOT NULL
);


CREATE TABLE bot_runtime_locks (
    name character varying(100) NOT NULL,
    owner_id character varying(200) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE client_blacklists (
    id character varying(25) NOT NULL,
    client_name character varying(100) NOT NULL,
    normalized_client_name character varying(100) NOT NULL,
    reason character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE device_actions (
    id character varying(25) NOT NULL,
    device_id character varying(100) DEFAULT ''::character varying NOT NULL,
    user_id character varying(25) DEFAULT ''::character varying NOT NULL,
    client_name character varying(100) DEFAULT ''::character varying NOT NULL,
    action character varying(50) NOT NULL,
    note character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    operator_id character varying(25)
);


CREATE TABLE email_verifications (
    id character varying(25) NOT NULL,
    email character varying(255) NOT NULL,
    code character varying(6) NOT NULL,
    ip character varying(45) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    type character varying(20) DEFAULT 'register'::character varying NOT NULL
);


CREATE TABLE emby_policy_sync_batches (
    id character varying(25) NOT NULL,
    plan_group_key character varying(50) NOT NULL,
    reason character varying(100) NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    total_count integer DEFAULT 0 NOT NULL,
    pending_count integer DEFAULT 0 NOT NULL,
    processing_count integer DEFAULT 0 NOT NULL,
    synced_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    created_by character varying(25),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone
);


CREATE TABLE emby_policy_sync_tasks (
    id character varying(25) NOT NULL,
    batch_id character varying(25),
    user_id character varying(25) NOT NULL,
    emby_id character varying(50) NOT NULL,
    plan_group_key character varying(50) NOT NULL,
    reason character varying(100) NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_error character varying(500),
    next_retry_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE failed_emby_async_ops (
    id character varying(25) NOT NULL,
    origin character varying(32) NOT NULL,
    origin_ref_id character varying(64) NOT NULL,
    emby_user_id character varying(64) NOT NULL,
    action character varying(20) NOT NULL,
    payload text,
    retries integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    last_error character varying(500),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE media_gap_scans (
    id character varying(25) NOT NULL,
    status character varying(20) NOT NULL,
    node_id character varying(64) NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    error_message character varying(500)
);


CREATE TABLE media_gaps (
    id character varying(25) NOT NULL,
    tmdb_id character varying(50) NOT NULL,
    emby_series_id character varying(50) DEFAULT ''::character varying NOT NULL,
    series_name character varying(255) DEFAULT ''::character varying NOT NULL,
    season integer NOT NULL,
    episode integer NOT NULL,
    air_date date NOT NULL,
    status character varying(20) DEFAULT 'MISSING'::character varying NOT NULL,
    search_snapshot text DEFAULT ''::text NOT NULL,
    dispatch_snapshot text DEFAULT ''::text NOT NULL,
    last_scanned_at timestamp with time zone,
    last_searched_at timestamp with time zone,
    requested_at timestamp with time zone,
    ingested_at timestamp with time zone,
    ignored_at timestamp with time zone,
    ignore_reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_dispatch_error character varying(500),
    ignore_reason_code character varying(50)
);


CREATE TABLE media_quality_caches (
    id character varying(25) NOT NULL,
    library_id character varying(100) NOT NULL,
    statistics text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    schema_version integer DEFAULT 1 NOT NULL,
    inflight_until timestamp with time zone
);


CREATE TABLE payments (
    id character varying(25) NOT NULL,
    user_id character varying(25) NOT NULL,
    plan_id character varying(25) NOT NULL,
    stripe_session_id character varying(255) NOT NULL,
    stripe_payment_intent_id character varying(255) DEFAULT ''::character varying NOT NULL,
    amount bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    days integer NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    checkout_url character varying(2048) DEFAULT ''::character varying NOT NULL,
    expires_at timestamp with time zone
);


CREATE TABLE plan_group_emby_policy_templates (
    plan_group_key character varying(50) NOT NULL,
    simultaneous_stream_limit integer DEFAULT 3 NOT NULL,
    enable_content_downloading boolean DEFAULT false NOT NULL,
    enable_live_tv_access boolean DEFAULT false NOT NULL,
    enable_sync_transcoding boolean DEFAULT false NOT NULL,
    enable_audio_playback_transcoding boolean DEFAULT false NOT NULL,
    enable_video_playback_transcoding boolean DEFAULT false NOT NULL,
    enable_playback_remuxing boolean DEFAULT true NOT NULL,
    enable_remote_access boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE plan_group_media_libraries (
    id character varying(25) NOT NULL,
    plan_group_key character varying(50) NOT NULL,
    library_id character varying(100) NOT NULL,
    library_name character varying(255) NOT NULL,
    library_type character varying(50) NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE plan_groups (
    key character varying(50) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE plans (
    id character varying(25) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    days integer NOT NULL,
    price bigint NOT NULL,
    currency character varying(3) DEFAULT 'usd'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    plan_group character varying(50) DEFAULT 'DEFAULT'::character varying NOT NULL
);


CREATE TABLE playback_rankings (
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
    batch_id character varying(32) DEFAULT ''::character varying NOT NULL,
    item_key character varying(128) DEFAULT ''::character varying NOT NULL,
    item_source_type character varying(32) DEFAULT ''::character varying NOT NULL
);


CREATE TABLE redemption_codes (
    id character varying(25) NOT NULL,
    code character varying(20) NOT NULL,
    max_uses bigint DEFAULT 1 NOT NULL,
    used_count bigint DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone,
    default_days bigint DEFAULT 30 NOT NULL,
    created_at timestamp with time zone,
    notes character varying(500) DEFAULT ''::character varying NOT NULL,
    registration_plan_group character varying(50) NOT NULL
);


CREATE TABLE redemptions (
    id character varying(25) NOT NULL,
    user_id character varying(25) NOT NULL,
    code character varying(20) NOT NULL,
    days bigint NOT NULL,
    created_at timestamp with time zone
);


CREATE TABLE settings (
    key character varying(100) NOT NULL,
    value text NOT NULL,
    updated_at timestamp with time zone,
    is_encrypted boolean DEFAULT false NOT NULL,
    updated_by_user_id character varying(25)
);


CREATE TABLE stripe_webhook_events (
    event_id character varying(64) NOT NULL,
    event_type character varying(64) NOT NULL,
    livemode boolean DEFAULT false NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    status character varying(20) DEFAULT 'received'::character varying NOT NULL,
    error_message character varying(500)
);


CREATE TABLE subscriptions (
    id character varying(25) NOT NULL,
    user_id character varying(25) NOT NULL,
    type character varying(10) NOT NULL,
    name character varying(255) NOT NULL,
    tmdb_id character varying(50) NOT NULL,
    poster_path character varying(500),
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    mp_error character varying(500),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    season integer DEFAULT 0 NOT NULL,
    reject_reason text,
    reviewed_at timestamp with time zone,
    ingested_at timestamp with time zone,
    retry_from_id character varying(25),
    ingest_progress character varying(50)
);


CREATE TABLE subscription_admin_notifications (
    id character varying(25) NOT NULL,
    subscription_id character varying(25) NOT NULL,
    admin_telegram_id bigint NOT NULL,
    chat_id bigint NOT NULL,
    message_id bigint,
    has_photo boolean DEFAULT false NOT NULL,
    delivery_status character varying(20) DEFAULT 'sent'::character varying NOT NULL,
    failure_reason character varying(500),
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


CREATE TABLE telegram_bind_codes (
    id character varying(25) NOT NULL,
    user_id character varying(25) NOT NULL,
    code character varying(6) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE tmdb_cache (
    id character varying(25) NOT NULL,
    cache_key character varying(255) NOT NULL,
    cache_value text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE tv_calendar_items (
    id character varying(25) NOT NULL,
    tmdb_id character varying(50) NOT NULL,
    series_id character varying(50) DEFAULT ''::character varying NOT NULL,
    season integer NOT NULL,
    episode integer NOT NULL,
    air_date date NOT NULL,
    episode_name character varying(255) DEFAULT ''::character varying NOT NULL,
    overview text DEFAULT ''::text NOT NULL,
    status character varying(20) DEFAULT 'upcoming'::character varying NOT NULL,
    emby_item_id character varying(50) DEFAULT ''::character varying NOT NULL,
    last_checked timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE tv_calendar_sources (
    id character varying(25) NOT NULL,
    tmdb_id character varying(50) NOT NULL,
    series_id character varying(50) DEFAULT ''::character varying NOT NULL,
    show_name character varying(255) DEFAULT ''::character varying NOT NULL,
    poster_url character varying(500) DEFAULT ''::character varying NOT NULL,
    overview text DEFAULT ''::text NOT NULL,
    emby_status character varying(20) DEFAULT 'continuing'::character varying NOT NULL,
    last_synced_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_episode_ingested_at timestamp with time zone,
    last_full_sync_at timestamp with time zone,
    last_correction_at timestamp with time zone
);


CREATE TABLE tv_calendar_subscriptions (
    id character varying(25) NOT NULL,
    user_id character varying(25) NOT NULL,
    tmdb_id character varying(50) NOT NULL,
    show_name character varying(255) NOT NULL,
    poster_url character varying(500) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE user_media_library_preferences (
    id character varying(25) NOT NULL,
    user_id character varying(25) NOT NULL,
    library_id character varying(100) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


CREATE TABLE users (
    id character varying(25) NOT NULL,
    username character varying(50) NOT NULL,
    role character varying(10) DEFAULT 'user'::character varying NOT NULL,
    password text,
    email character varying(255),
    emby_id character varying(50),
    emby_disabled boolean DEFAULT false NOT NULL,
    expires_at timestamp with time zone,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    telegram_id bigint,
    plan_group character varying(50),
    password_reset_required boolean DEFAULT false NOT NULL,
    emby_access_disabled boolean DEFAULT false NOT NULL
);


-- ┌─ 主键约束 ─────────────────────────────────────────────────────────

ALTER TABLE ONLY bot_pending_reject_requests
    ADD CONSTRAINT bot_pending_reject_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bot_runtime_locks
    ADD CONSTRAINT bot_runtime_locks_pkey PRIMARY KEY (name);

ALTER TABLE ONLY client_blacklists
    ADD CONSTRAINT client_blacklists_pkey PRIMARY KEY (id);

ALTER TABLE ONLY device_actions
    ADD CONSTRAINT device_actions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY emby_policy_sync_batches
    ADD CONSTRAINT emby_policy_sync_batches_pkey PRIMARY KEY (id);

ALTER TABLE ONLY emby_policy_sync_tasks
    ADD CONSTRAINT emby_policy_sync_tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY failed_emby_async_ops
    ADD CONSTRAINT failed_emby_async_ops_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media_gap_scans
    ADD CONSTRAINT media_gap_scans_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media_gaps
    ADD CONSTRAINT media_gaps_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media_quality_caches
    ADD CONSTRAINT media_quality_caches_pkey PRIMARY KEY (id);

ALTER TABLE ONLY payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY plan_group_emby_policy_templates
    ADD CONSTRAINT plan_group_emby_policy_templates_pkey PRIMARY KEY (plan_group_key);

ALTER TABLE ONLY plan_group_media_libraries
    ADD CONSTRAINT plan_group_media_libraries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY plan_groups
    ADD CONSTRAINT plan_groups_pkey PRIMARY KEY (key);

ALTER TABLE ONLY plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);

ALTER TABLE ONLY playback_rankings
    ADD CONSTRAINT playback_rankings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY redemption_codes
    ADD CONSTRAINT redemption_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY redemptions
    ADD CONSTRAINT redemptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);

ALTER TABLE ONLY stripe_webhook_events
    ADD CONSTRAINT stripe_webhook_events_pkey PRIMARY KEY (event_id);

ALTER TABLE ONLY subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY subscription_admin_notifications
    ADD CONSTRAINT subscription_admin_notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY telegram_bind_codes
    ADD CONSTRAINT telegram_bind_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tmdb_cache
    ADD CONSTRAINT tmdb_cache_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tv_calendar_items
    ADD CONSTRAINT tv_calendar_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tv_calendar_sources
    ADD CONSTRAINT tv_calendar_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tv_calendar_subscriptions
    ADD CONSTRAINT tv_calendar_subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_media_library_preferences
    ADD CONSTRAINT user_media_library_preferences_pkey PRIMARY KEY (id);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


-- ┌─ 索引（按字典序） ──────────────────────────────────────────────────

CREATE INDEX idx_bot_pending_reject_requests_chat ON bot_pending_reject_requests USING btree (chat_id, expires_at);

CREATE INDEX idx_bot_pending_reject_requests_expires ON bot_pending_reject_requests USING btree (expires_at);

CREATE INDEX idx_bot_runtime_locks_expires ON bot_runtime_locks USING btree (expires_at);

CREATE INDEX idx_device_actions_device_id ON device_actions USING btree (device_id);

CREATE INDEX idx_device_actions_operator ON device_actions USING btree (operator_id) WHERE (operator_id IS NOT NULL);

CREATE INDEX idx_device_actions_user_id ON device_actions USING btree (user_id);

CREATE INDEX idx_email_verifications_email ON email_verifications USING btree (email);

CREATE INDEX idx_email_verifications_ip ON email_verifications USING btree (ip);

CREATE INDEX idx_email_verifications_type ON email_verifications USING btree (type);

CREATE INDEX idx_emby_policy_sync_batches_group ON emby_policy_sync_batches USING btree (plan_group_key);

CREATE INDEX idx_emby_policy_sync_batches_status ON emby_policy_sync_batches USING btree (status);

CREATE INDEX idx_emby_policy_sync_tasks_batch ON emby_policy_sync_tasks USING btree (batch_id);

CREATE INDEX idx_emby_policy_sync_tasks_group ON emby_policy_sync_tasks USING btree (plan_group_key);

CREATE INDEX idx_emby_policy_sync_tasks_status_retry ON emby_policy_sync_tasks USING btree (status, next_retry_at);

CREATE INDEX idx_emby_policy_sync_tasks_user ON emby_policy_sync_tasks USING btree (user_id);

CREATE INDEX idx_failed_emby_async_ops_next ON failed_emby_async_ops USING btree (next_attempt_at, retries);

CREATE INDEX idx_failed_emby_async_ops_origin ON failed_emby_async_ops USING btree (origin, origin_ref_id);

CREATE INDEX idx_media_gap_scans_started ON media_gap_scans USING btree (started_at);

CREATE INDEX idx_media_gaps_emby_series_id ON media_gaps USING btree (emby_series_id);

CREATE INDEX idx_media_gaps_status_air_date ON media_gaps USING btree (status, air_date);

CREATE INDEX idx_media_quality_caches_expires_at ON media_quality_caches USING btree (expires_at);

CREATE INDEX idx_media_quality_caches_inflight ON media_quality_caches USING btree (inflight_until) WHERE (inflight_until IS NOT NULL);

CREATE INDEX idx_payments_expires_at ON payments USING btree (expires_at);

CREATE INDEX idx_payments_plan_id ON payments USING btree (plan_id);

CREATE UNIQUE INDEX idx_payments_stripe_session_id ON payments USING btree (stripe_session_id) WHERE ((stripe_session_id)::text <> ''::text);

CREATE INDEX idx_payments_user_id ON payments USING btree (user_id);

CREATE INDEX idx_plan_group_media_libraries_group ON plan_group_media_libraries USING btree (plan_group_key);

CREATE INDEX idx_plan_groups_is_default ON plan_groups USING btree (is_default);

CREATE INDEX idx_plan_groups_sort_order ON plan_groups USING btree (sort_order);

CREATE INDEX idx_plans_active_sort ON plans USING btree (is_active, sort_order);

CREATE INDEX idx_plans_plan_group ON plans USING btree (plan_group);

CREATE INDEX idx_ranking_batch ON playback_rankings USING btree (batch_id, category, rank);

CREATE INDEX idx_ranking_item ON playback_rankings USING btree (period, category, item_key, period_start, period_end);

CREATE INDEX idx_ranking_lookup ON playback_rankings USING btree (period, category, snapshot_at);

CREATE INDEX idx_ranking_period_window ON playback_rankings USING btree (period, period_start, period_end, snapshot_at);

CREATE UNIQUE INDEX idx_redemption_codes_code ON redemption_codes USING btree (code);

CREATE INDEX idx_redemption_codes_registration_plan_group ON redemption_codes USING btree (registration_plan_group);

CREATE INDEX idx_redemptions_user_id ON redemptions USING btree (user_id);

CREATE INDEX idx_stripe_webhook_events_received ON stripe_webhook_events USING btree (received_at);

CREATE INDEX idx_subscription_admin_notifications_admin ON subscription_admin_notifications USING btree (admin_telegram_id);

CREATE INDEX idx_subscription_admin_notifications_subscription ON subscription_admin_notifications USING btree (subscription_id);

CREATE INDEX idx_subscriptions_retry_from_id ON subscriptions USING btree (retry_from_id);

CREATE INDEX idx_subscriptions_user_id ON subscriptions USING btree (user_id);

CREATE UNIQUE INDEX idx_telegram_bind_codes_code ON telegram_bind_codes USING btree (code);

CREATE INDEX idx_telegram_bind_codes_user_id ON telegram_bind_codes USING btree (user_id);

CREATE INDEX idx_tmdb_cache_expires_at ON tmdb_cache USING btree (expires_at);

CREATE INDEX idx_tv_calendar_items_air_date ON tv_calendar_items USING btree (air_date);

CREATE INDEX idx_tv_calendar_items_series_id ON tv_calendar_items USING btree (series_id);

CREATE INDEX idx_tv_calendar_items_tmdb_id ON tv_calendar_items USING btree (tmdb_id);

CREATE INDEX idx_tv_calendar_sources_last_episode_ingested_at ON tv_calendar_sources USING btree (last_episode_ingested_at);

CREATE INDEX idx_tv_calendar_sources_series_id ON tv_calendar_sources USING btree (series_id);

CREATE INDEX idx_tv_calendar_subscriptions_tmdb_id ON tv_calendar_subscriptions USING btree (tmdb_id);

CREATE INDEX idx_tv_calendar_subscriptions_user_id ON tv_calendar_subscriptions USING btree (user_id);

CREATE INDEX idx_user_media_library_preferences_user ON user_media_library_preferences USING btree (user_id);

CREATE UNIQUE INDEX idx_users_email ON users USING btree (email);

CREATE INDEX idx_users_plan_group ON users USING btree (plan_group);

CREATE UNIQUE INDEX idx_users_telegram_id ON users USING btree (telegram_id) WHERE (telegram_id IS NOT NULL);

CREATE UNIQUE INDEX idx_users_username ON users USING btree (username);

CREATE UNIQUE INDEX uk_media_gap_episode ON media_gaps USING btree (tmdb_id, season, episode);

CREATE UNIQUE INDEX uniq_users_emby_id ON users USING btree (emby_id) WHERE ((emby_id IS NOT NULL) AND ((emby_id)::text <> ''::text));

CREATE UNIQUE INDEX uq_client_blacklists_normalized_client_name ON client_blacklists USING btree (normalized_client_name);

CREATE UNIQUE INDEX uq_emby_policy_sync_tasks_user_active ON emby_policy_sync_tasks USING btree (user_id) WHERE ((status)::text = ANY ((ARRAY['pending'::character varying, 'processing'::character varying])::text[]));

CREATE UNIQUE INDEX uq_media_quality_caches_library_id ON media_quality_caches USING btree (library_id);

CREATE UNIQUE INDEX uq_payments_pending_user_plan ON payments USING btree (user_id, plan_id) WHERE ((status)::text = 'pending'::text);

CREATE UNIQUE INDEX uq_plan_group_media_libraries_group_library ON plan_group_media_libraries USING btree (plan_group_key, library_id);

CREATE UNIQUE INDEX uq_plan_groups_default_true ON plan_groups USING btree (is_default) WHERE (is_default = true);

CREATE UNIQUE INDEX uq_playback_rankings_period ON playback_rankings USING btree (period, period_start, period_end);

CREATE UNIQUE INDEX uq_redemptions_user_code ON redemptions USING btree (user_id, code);

CREATE UNIQUE INDEX uq_subscription_admin_notifications_message ON subscription_admin_notifications USING btree (chat_id, message_id) WHERE (message_id IS NOT NULL);

CREATE UNIQUE INDEX uq_subscriptions_active_media ON subscriptions USING btree (type, tmdb_id, season) WHERE ((status)::text = ANY ((ARRAY['PENDING'::character varying, 'APPROVED'::character varying, 'INGESTED'::character varying])::text[]));

CREATE UNIQUE INDEX uq_telegram_bind_codes_user ON telegram_bind_codes USING btree (user_id);

CREATE UNIQUE INDEX uq_tmdb_cache_cache_key ON tmdb_cache USING btree (cache_key);

CREATE UNIQUE INDEX uq_tv_calendar_items_tmdb_season_episode ON tv_calendar_items USING btree (tmdb_id, season, episode);

CREATE UNIQUE INDEX uq_tv_calendar_sources_tmdb_id ON tv_calendar_sources USING btree (tmdb_id);

CREATE UNIQUE INDEX uq_tv_calendar_subscriptions_user_tmdb ON tv_calendar_subscriptions USING btree (user_id, tmdb_id);

CREATE UNIQUE INDEX uq_user_media_library_preferences_user_library ON user_media_library_preferences USING btree (user_id, library_id);

CREATE UNIQUE INDEX uq_users_email_lower ON users USING btree (lower((email)::text)) WHERE ((email IS NOT NULL) AND ((email)::text <> ''::text));

CREATE UNIQUE INDEX uq_users_telegram_id ON users USING btree (telegram_id) WHERE (telegram_id IS NOT NULL);

CREATE UNIQUE INDEX uq_users_username_lower ON users USING btree (lower((username)::text));


-- ┌─ 默认 seed 数据 ───────────────────────────────────────────────────

INSERT INTO settings (key, value, updated_at, is_encrypted, updated_by_user_id) VALUES
  ('default_trial_days',           '7',     now(), false, NULL),
  ('registration_mode',            'open',  now(), false, NULL),
  ('notify_group_link',            '',      now(), false, NULL),
  ('email_verification',           'false', now(), false, NULL),
  ('stripe_allowed_payment_methods','',     now(), false, NULL),
  ('telegram_approval_admin_ids',  '',      now(), false, NULL)
ON CONFLICT (key) DO NOTHING;

INSERT INTO plan_groups (key, name, description, is_default, sort_order, created_at, updated_at)
VALUES ('DEFAULT', '默认分组', '系统默认套餐分组', true, 10, now(), now())
ON CONFLICT (key) DO NOTHING;

INSERT INTO plan_group_emby_policy_templates (
  plan_group_key,
  simultaneous_stream_limit,
  enable_content_downloading,
  enable_live_tv_access,
  enable_sync_transcoding,
  enable_audio_playback_transcoding,
  enable_video_playback_transcoding,
  enable_playback_remuxing,
  enable_remote_access,
  created_at,
  updated_at
)
VALUES ('DEFAULT', 3, false, false, false, false, false, true, true, now(), now())
ON CONFLICT (plan_group_key) DO NOTHING;
