-- Add plan-group routing/transfer limits and user-owned 115 playback accounts.
-- Existing plan groups receive the product default `personal` through ADD COLUMN DEFAULT.
-- Existing administrator playback accounts keep their credential and target ID, but an
-- incomplete enabled account is disabled because path and concurrency cannot be inferred.
-- No user-owned credential, target path, or concurrency value is guessed or backfilled.
-- The migration is idempotent and safe to re-run without overwriting an explicit `system` mode.

ALTER TABLE plan_groups
    ADD COLUMN IF NOT EXISTS p115_playback_mode VARCHAR(20) NOT NULL DEFAULT 'personal',
    ADD COLUMN IF NOT EXISTS p115_transfer_hourly_limit INTEGER NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS p115_transfer_daily_limit INTEGER NOT NULL DEFAULT 10;

ALTER TABLE plan_groups
    ALTER COLUMN p115_playback_mode SET DEFAULT 'personal',
    ALTER COLUMN p115_transfer_hourly_limit SET DEFAULT 5,
    ALTER COLUMN p115_transfer_daily_limit SET DEFAULT 10;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_plan_groups_p115_playback_mode'
          AND conrelid = 'plan_groups'::regclass
    ) THEN
        ALTER TABLE plan_groups
            ADD CONSTRAINT ck_plan_groups_p115_playback_mode
            CHECK (p115_playback_mode IN ('personal', 'system'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_plan_groups_p115_transfer_hourly_limit'
          AND conrelid = 'plan_groups'::regclass
    ) THEN
        ALTER TABLE plan_groups
            ADD CONSTRAINT ck_plan_groups_p115_transfer_hourly_limit
            CHECK (p115_transfer_hourly_limit BETWEEN 1 AND 100);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_plan_groups_p115_transfer_daily_limit'
          AND conrelid = 'plan_groups'::regclass
    ) THEN
        ALTER TABLE plan_groups
            ADD CONSTRAINT ck_plan_groups_p115_transfer_daily_limit
            CHECK (p115_transfer_daily_limit BETWEEN 1 AND 1000);
    END IF;
END $$;

ALTER TABLE p115_accounts
    ADD COLUMN IF NOT EXISTS owner_user_id VARCHAR(25),
    ADD COLUMN IF NOT EXISTS target_parent_path VARCHAR(4096),
    ADD COLUMN IF NOT EXISTS max_concurrent_streams INTEGER;

ALTER TABLE p115_accounts
    ALTER COLUMN cookie_ciphertext DROP NOT NULL,
    ALTER COLUMN app_type DROP NOT NULL,
    ALTER COLUMN user_agent DROP NOT NULL;

-- A legacy administrator playback account has no path snapshot or concurrency value.
-- Preserve its credential/target ID and require the administrator to configure it explicitly.
UPDATE p115_accounts
SET enabled = false,
    updated_at = CURRENT_TIMESTAMP
WHERE role = 'playback'
  AND enabled = true
  AND (
      target_parent_path IS NULL
      OR btrim(target_parent_path) = ''
      OR max_concurrent_streams IS NULL
      OR max_concurrent_streams <= 0
  );

ALTER TABLE p115_accounts
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_status,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_target_parent,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_required_values,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_source_location,
    DROP CONSTRAINT IF EXISTS fk_p115_accounts_owner_user,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_owner_scope,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_credential_state,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_location_state,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_max_concurrent_streams,
    DROP CONSTRAINT IF EXISTS ck_p115_accounts_enabled_complete;

ALTER TABLE p115_accounts
    ADD CONSTRAINT fk_p115_accounts_owner_user
        FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    ADD CONSTRAINT ck_p115_accounts_status
        CHECK (status IN ('pending', 'active', 'expired', 'error', 'cooling_down', 'revoked')),
    ADD CONSTRAINT ck_p115_accounts_owner_scope
        CHECK (
            owner_user_id IS NULL
            OR (role = 'playback' AND status <> 'revoked')
        ),
    ADD CONSTRAINT ck_p115_accounts_credential_state
        CHECK (
            (
                status = 'revoked'
                AND enabled = false
                AND owner_user_id IS NULL
                AND provider_user_id IS NULL
                AND cookie_ciphertext IS NULL
                AND app_type IS NULL
                AND user_agent IS NULL
                AND emby_path_prefix IS NULL
                AND source_root_id IS NULL
                AND target_parent_id IS NULL
                AND target_parent_path IS NULL
                AND max_concurrent_streams IS NULL
                AND last_validated_at IS NULL
                AND last_succeeded_at IS NULL
                AND cooldown_until IS NULL
                AND last_error_code IS NULL
                AND last_error_message IS NULL
            )
            OR (
                status <> 'revoked'
                AND cookie_ciphertext IS NOT NULL
                AND btrim(cookie_ciphertext) <> ''
                AND app_type IS NOT NULL
                AND btrim(app_type) <> ''
                AND user_agent IS NOT NULL
                AND btrim(user_agent) <> ''
            )
        ),
    ADD CONSTRAINT ck_p115_accounts_location_state
        CHECK (
            (
                role = 'source'
                AND target_parent_id IS NULL
                AND target_parent_path IS NULL
                AND max_concurrent_streams IS NULL
                AND (
                    (emby_path_prefix IS NULL AND source_root_id IS NULL)
                    OR (
                        emby_path_prefix IS NOT NULL
                        AND btrim(emby_path_prefix) <> ''
                        AND left(emby_path_prefix, 1) = '/'
                        AND emby_path_prefix <> '/'
                        AND right(emby_path_prefix, 1) <> '/'
                        AND source_root_id IS NOT NULL
                        AND source_root_id ~ '^(0|[1-9][0-9]*)$'
                    )
                )
            )
            OR (
                role = 'playback'
                AND emby_path_prefix IS NULL
                AND source_root_id IS NULL
                AND (
                    owner_user_id IS NULL
                    OR (
                        (target_parent_id IS NULL AND target_parent_path IS NULL)
                        OR (
                            target_parent_id IS NOT NULL
                            AND btrim(target_parent_id) <> ''
                            AND target_parent_path IS NOT NULL
                            AND btrim(target_parent_path) <> ''
                        )
                    )
                )
            )
        ),
    ADD CONSTRAINT ck_p115_accounts_max_concurrent_streams
        CHECK (
            max_concurrent_streams IS NULL
            OR (role = 'playback' AND status <> 'revoked' AND max_concurrent_streams > 0)
        ),
    ADD CONSTRAINT ck_p115_accounts_enabled_complete
        CHECK (
            enabled = false
            OR (
                status <> 'pending'
                AND status <> 'expired'
                AND status <> 'revoked'
                AND provider_user_id IS NOT NULL
                AND btrim(provider_user_id) <> ''
                AND (
                    (
                        role = 'source'
                        AND emby_path_prefix IS NOT NULL
                        AND btrim(emby_path_prefix) <> ''
                        AND source_root_id IS NOT NULL
                        AND btrim(source_root_id) <> ''
                    )
                    OR (
                        role = 'playback'
                        AND target_parent_id IS NOT NULL
                        AND btrim(target_parent_id) <> ''
                        AND target_parent_path IS NOT NULL
                        AND btrim(target_parent_path) <> ''
                        AND max_concurrent_streams IS NOT NULL
                        AND max_concurrent_streams > 0
                    )
                )
            )
        );

DROP INDEX IF EXISTS uq_p115_accounts_enabled_role;
DROP INDEX IF EXISTS uq_p115_accounts_enabled_provider_user;

CREATE UNIQUE INDEX IF NOT EXISTS uq_p115_accounts_enabled_shared_role
    ON p115_accounts (role)
    WHERE enabled = true
      AND owner_user_id IS NULL
      AND status <> 'revoked';

CREATE UNIQUE INDEX IF NOT EXISTS uq_p115_accounts_non_revoked_provider_user
    ON p115_accounts (provider_user_id)
    WHERE status <> 'revoked'
      AND provider_user_id IS NOT NULL
      AND btrim(provider_user_id) <> '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_p115_accounts_current_owner
    ON p115_accounts (owner_user_id)
    WHERE owner_user_id IS NOT NULL
      AND status <> 'revoked';

CREATE INDEX IF NOT EXISTS idx_p115_accounts_owner_status
    ON p115_accounts (owner_user_id, status);
