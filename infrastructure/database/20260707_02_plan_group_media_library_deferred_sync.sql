ALTER TABLE plan_groups
    ADD COLUMN IF NOT EXISTS media_library_template_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS applied_media_library_template_version BIGINT NOT NULL DEFAULT 1;
