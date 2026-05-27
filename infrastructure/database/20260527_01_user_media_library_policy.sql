-- User media library management first backend slice.
-- Adds group media library templates, group Emby policy templates, user library preferences,
-- Emby policy sync batch/task metadata, and users.emby_access_disabled.
-- Idempotent; no foreign keys are created by design. Cross-table integrity is enforced in services.

DO $$
DECLARE
  default_group_count integer;
  default_group_key varchar(50);
BEGIN
  SELECT count(*), max(key)
    INTO default_group_count, default_group_key
    FROM plan_groups
   WHERE is_default = true;

  IF default_group_count <> 1 THEN
    RAISE EXCEPTION 'user media library policy migration requires exactly one default plan_group, found %', default_group_count;
  END IF;

  UPDATE users
     SET plan_group = default_group_key
   WHERE plan_group IS NULL;

  UPDATE redemption_codes
     SET registration_plan_group = default_group_key
   WHERE registration_plan_group IS NULL;

  IF EXISTS (
    SELECT 1
      FROM plans p
      LEFT JOIN plan_groups pg ON pg.key = p.plan_group
     WHERE pg.key IS NULL
  ) THEN
    RAISE EXCEPTION 'plans.plan_group contains values not present in plan_groups';
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS plan_group_media_libraries (
  id varchar(25) PRIMARY KEY,
  plan_group_key varchar(50) NOT NULL,
  library_id varchar(100) NOT NULL,
  library_name varchar(255) NOT NULL,
  library_type varchar(50) NOT NULL,
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_plan_group_media_libraries_group_library
  ON plan_group_media_libraries (plan_group_key, library_id);

CREATE INDEX IF NOT EXISTS idx_plan_group_media_libraries_group
  ON plan_group_media_libraries (plan_group_key);

CREATE TABLE IF NOT EXISTS plan_group_emby_policy_templates (
  plan_group_key varchar(50) PRIMARY KEY,
  simultaneous_stream_limit integer NOT NULL DEFAULT 3,
  enable_content_downloading boolean NOT NULL DEFAULT false,
  enable_live_tv_access boolean NOT NULL DEFAULT false,
  enable_sync_transcoding boolean NOT NULL DEFAULT false,
  enable_audio_playback_transcoding boolean NOT NULL DEFAULT false,
  enable_video_playback_transcoding boolean NOT NULL DEFAULT false,
  enable_playback_remuxing boolean NOT NULL DEFAULT true,
  enable_remote_access boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO plan_group_emby_policy_templates (
  plan_group_key,
  simultaneous_stream_limit,
  enable_content_downloading,
  enable_live_tv_access,
  enable_sync_transcoding,
  enable_audio_playback_transcoding,
  enable_video_playback_transcoding,
  enable_playback_remuxing,
  enable_remote_access
)
SELECT
  pg.key,
  3,
  false,
  false,
  false,
  false,
  false,
  true,
  true
FROM plan_groups pg
WHERE NOT EXISTS (
  SELECT 1
    FROM plan_group_emby_policy_templates t
   WHERE t.plan_group_key = pg.key
);

CREATE TABLE IF NOT EXISTS user_media_library_preferences (
  id varchar(25) PRIMARY KEY,
  user_id varchar(25) NOT NULL,
  library_id varchar(100) NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_media_library_preferences_user_library
  ON user_media_library_preferences (user_id, library_id);

CREATE INDEX IF NOT EXISTS idx_user_media_library_preferences_user
  ON user_media_library_preferences (user_id);

CREATE TABLE IF NOT EXISTS emby_policy_sync_batches (
  id varchar(25) PRIMARY KEY,
  plan_group_key varchar(50) NOT NULL,
  reason varchar(100) NOT NULL,
  status varchar(20) NOT NULL DEFAULT 'pending',
  total_count integer NOT NULL DEFAULT 0,
  pending_count integer NOT NULL DEFAULT 0,
  processing_count integer NOT NULL DEFAULT 0,
  synced_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  created_by varchar(25),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_emby_policy_sync_batches_group
  ON emby_policy_sync_batches (plan_group_key);

CREATE INDEX IF NOT EXISTS idx_emby_policy_sync_batches_status
  ON emby_policy_sync_batches (status);

CREATE TABLE IF NOT EXISTS emby_policy_sync_tasks (
  id varchar(25) PRIMARY KEY,
  batch_id varchar(25),
  user_id varchar(25) NOT NULL,
  emby_id varchar(50) NOT NULL,
  plan_group_key varchar(50) NOT NULL,
  reason varchar(100) NOT NULL,
  status varchar(20) NOT NULL DEFAULT 'pending',
  attempts integer NOT NULL DEFAULT 0,
  last_error varchar(500),
  next_retry_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_emby_policy_sync_tasks_batch
  ON emby_policy_sync_tasks (batch_id);

CREATE INDEX IF NOT EXISTS idx_emby_policy_sync_tasks_user
  ON emby_policy_sync_tasks (user_id);

CREATE INDEX IF NOT EXISTS idx_emby_policy_sync_tasks_group
  ON emby_policy_sync_tasks (plan_group_key);

CREATE INDEX IF NOT EXISTS idx_emby_policy_sync_tasks_status_retry
  ON emby_policy_sync_tasks (status, next_retry_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_emby_policy_sync_tasks_user_active
  ON emby_policy_sync_tasks (user_id)
  WHERE status IN ('pending', 'processing');

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS emby_access_disabled boolean NOT NULL DEFAULT false;

UPDATE users
   SET emby_access_disabled = true
 WHERE emby_disabled = true
   AND (expires_at IS NULL OR expires_at >= now());
