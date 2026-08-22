-- Persist one-way Emby AccessToken mappings for Playback Gateway identity.
-- No token plaintext is stored and no historical token can be backfilled.
-- The migration is idempotent and safe to re-run.

CREATE TABLE IF NOT EXISTS emby_access_tokens (
    id VARCHAR(25) PRIMARY KEY,
    server_id VARCHAR(64) NOT NULL,
    token_hash BYTEA NOT NULL,
    emby_user_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(25) REFERENCES users(id) ON DELETE SET NULL,
    device_id VARCHAR(256) NOT NULL DEFAULT '',
    client_name VARCHAR(128) NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ,
    revoked_reason VARCHAR(100),
    revoked_by VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_emby_access_tokens_identity'
          AND conrelid = 'emby_access_tokens'::regclass
    ) THEN
        ALTER TABLE emby_access_tokens
            ADD CONSTRAINT ck_emby_access_tokens_identity
            CHECK (
                btrim(server_id) <> ''
                AND octet_length(token_hash) = 32
                AND btrim(emby_user_id) <> ''
                AND (user_id IS NOT NULL OR revoked_at IS NOT NULL)
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_emby_access_tokens_revocation'
          AND conrelid = 'emby_access_tokens'::regclass
    ) THEN
        ALTER TABLE emby_access_tokens
            ADD CONSTRAINT ck_emby_access_tokens_revocation
            CHECK (
                (
                    revoked_at IS NULL
                    AND revoked_reason IS NULL
                    AND revoked_by IS NULL
                    AND user_id IS NOT NULL
                )
                OR (
                    revoked_at IS NOT NULL
                    AND revoked_reason IS NOT NULL
                    AND btrim(revoked_reason) <> ''
                    AND revoked_by IS NOT NULL
                    AND btrim(revoked_by) <> ''
                )
            );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_emby_access_tokens_server_hash
    ON emby_access_tokens (server_id, token_hash);

CREATE INDEX IF NOT EXISTS idx_emby_access_tokens_user_active
    ON emby_access_tokens (user_id)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_emby_access_tokens_device_active
    ON emby_access_tokens (server_id, user_id, device_id)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_emby_access_tokens_last_seen
    ON emby_access_tokens (last_seen_at);
