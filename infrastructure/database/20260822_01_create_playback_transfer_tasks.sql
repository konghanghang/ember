-- Persist retained 115 playback transfer attempts and their reusable provenance.
-- No backfill is required. Existing playback files remain external preexisting files.
-- The migration is idempotent and safe to re-run.

CREATE TABLE IF NOT EXISTS playback_transfer_tasks (
    id VARCHAR(25) PRIMARY KEY,
    source_account_id VARCHAR(25) NOT NULL REFERENCES p115_accounts(id) ON DELETE RESTRICT,
    playback_account_id VARCHAR(25) NOT NULL REFERENCES p115_accounts(id) ON DELETE RESTRICT,
    sha1 CHAR(40) NOT NULL,
    size BIGINT NOT NULL,
    file_name VARCHAR(1024) NOT NULL,
    target_parent_id VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    target_file_id VARCHAR(64),
    target_pick_code VARCHAR(128),
    attempt_count INTEGER NOT NULL DEFAULT 1,
    last_error_code VARCHAR(100),
    last_error_message VARCHAR(500),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    last_accessed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_playback_transfer_tasks_status'
          AND conrelid = 'playback_transfer_tasks'::regclass
    ) THEN
        ALTER TABLE playback_transfer_tasks
            ADD CONSTRAINT ck_playback_transfer_tasks_status
            CHECK (status IN (
                'pending', 'initializing', 'challenging',
                'verifying', 'succeeded', 'failed'
            ));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_playback_transfer_tasks_identity'
          AND conrelid = 'playback_transfer_tasks'::regclass
    ) THEN
        ALTER TABLE playback_transfer_tasks
            ADD CONSTRAINT ck_playback_transfer_tasks_identity
            CHECK (
                source_account_id <> playback_account_id
                AND sha1 ~ '^[0-9A-F]{40}$'
                AND size > 0
                AND btrim(file_name) <> ''
                AND btrim(target_parent_id) <> ''
                AND attempt_count > 0
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_playback_transfer_tasks_terminal'
          AND conrelid = 'playback_transfer_tasks'::regclass
    ) THEN
        ALTER TABLE playback_transfer_tasks
            ADD CONSTRAINT ck_playback_transfer_tasks_terminal
            CHECK (
                (
                    status = 'succeeded'
                    AND target_file_id IS NOT NULL AND btrim(target_file_id) <> ''
                    AND target_pick_code IS NOT NULL AND btrim(target_pick_code) <> ''
                    AND completed_at IS NOT NULL
                    AND last_accessed_at IS NOT NULL
                    AND last_error_code IS NULL
                    AND last_error_message IS NULL
                )
                OR (
                    status = 'failed'
                    AND completed_at IS NOT NULL
                    AND last_error_code IS NOT NULL AND btrim(last_error_code) <> ''
                )
                OR (
                    status IN ('pending', 'initializing', 'challenging', 'verifying')
                    AND completed_at IS NULL
                )
            );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_playback_transfer_tasks_active_content
    ON playback_transfer_tasks (playback_account_id, sha1, size)
    WHERE status IN ('pending', 'initializing', 'challenging', 'verifying');

CREATE INDEX IF NOT EXISTS idx_playback_transfer_tasks_content_lookup
    ON playback_transfer_tasks (playback_account_id, sha1, size, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_playback_transfer_tasks_last_accessed
    ON playback_transfer_tasks (last_accessed_at)
    WHERE status = 'succeeded';
