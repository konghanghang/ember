-- Attach the phase-one Emby mount prefix and 115 source root to the source account.
-- Existing accounts are not guessed or backfilled; runtime enable/load gates require explicit configuration.
-- The migration is idempotent and safe to re-run.

ALTER TABLE p115_accounts
    ADD COLUMN IF NOT EXISTS emby_path_prefix VARCHAR(4096),
    ADD COLUMN IF NOT EXISTS source_root_id VARCHAR(64);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_p115_accounts_source_location'
          AND conrelid = 'p115_accounts'::regclass
    ) THEN
        ALTER TABLE p115_accounts
            ADD CONSTRAINT ck_p115_accounts_source_location
            CHECK (
                (
                    role = 'source'
                    AND target_parent_id IS NULL
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
                    AND target_parent_id IS NOT NULL
                    AND btrim(target_parent_id) <> ''
                    AND emby_path_prefix IS NULL
                    AND source_root_id IS NULL
                )
            );
    END IF;
END $$;
