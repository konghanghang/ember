-- Create the administrator-managed 115 account store.
-- Cookie plaintext is never persisted; cookie_ciphertext uses CONFIG_ENCRYPTION_KEY.
-- No backfill is required. The migration is safe to re-run.

CREATE TABLE IF NOT EXISTS p115_accounts (
    id VARCHAR(25) PRIMARY KEY,
    role VARCHAR(20) NOT NULL,
    alias VARCHAR(100) NOT NULL,
    auth_mode VARCHAR(20) NOT NULL DEFAULT 'legacy_cookie',
    provider_user_id VARCHAR(64),
    cookie_ciphertext TEXT NOT NULL,
    app_type VARCHAR(32) NOT NULL,
    user_agent VARCHAR(512) NOT NULL,
    target_parent_id VARCHAR(64),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    enabled BOOLEAN NOT NULL DEFAULT false,
    last_validated_at TIMESTAMPTZ,
    last_succeeded_at TIMESTAMPTZ,
    cooldown_until TIMESTAMPTZ,
    last_error_code VARCHAR(100),
    last_error_message VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_p115_accounts_role'
          AND conrelid = 'p115_accounts'::regclass
    ) THEN
        ALTER TABLE p115_accounts
            ADD CONSTRAINT ck_p115_accounts_role
            CHECK (role IN ('source', 'playback'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_p115_accounts_auth_mode'
          AND conrelid = 'p115_accounts'::regclass
    ) THEN
        ALTER TABLE p115_accounts
            ADD CONSTRAINT ck_p115_accounts_auth_mode
            CHECK (auth_mode IN ('legacy_cookie'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_p115_accounts_status'
          AND conrelid = 'p115_accounts'::regclass
    ) THEN
        ALTER TABLE p115_accounts
            ADD CONSTRAINT ck_p115_accounts_status
            CHECK (status IN ('pending', 'active', 'expired', 'error', 'cooling_down'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_p115_accounts_target_parent'
          AND conrelid = 'p115_accounts'::regclass
    ) THEN
        ALTER TABLE p115_accounts
            ADD CONSTRAINT ck_p115_accounts_target_parent
            CHECK (
                (role = 'source' AND target_parent_id IS NULL)
                OR
                (role = 'playback' AND target_parent_id IS NOT NULL AND btrim(target_parent_id) <> '')
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_p115_accounts_required_values'
          AND conrelid = 'p115_accounts'::regclass
    ) THEN
        ALTER TABLE p115_accounts
            ADD CONSTRAINT ck_p115_accounts_required_values
            CHECK (
                btrim(alias) <> ''
                AND btrim(cookie_ciphertext) <> ''
                AND btrim(app_type) <> ''
                AND btrim(user_agent) <> ''
            );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_p115_accounts_enabled_role
    ON p115_accounts (role)
    WHERE enabled = true;

CREATE UNIQUE INDEX IF NOT EXISTS uq_p115_accounts_enabled_provider_user
    ON p115_accounts (provider_user_id)
    WHERE enabled = true
      AND provider_user_id IS NOT NULL
      AND btrim(provider_user_id) <> '';

CREATE INDEX IF NOT EXISTS idx_p115_accounts_status_cooldown
    ON p115_accounts (status, cooldown_until);
