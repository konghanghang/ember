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
