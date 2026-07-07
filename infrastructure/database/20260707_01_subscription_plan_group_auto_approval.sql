ALTER TABLE plan_groups
    ADD COLUMN IF NOT EXISTS subscription_auto_approve_daily_limit INTEGER NOT NULL DEFAULT 0;

UPDATE plan_groups
SET subscription_auto_approve_daily_limit = 0
WHERE subscription_auto_approve_daily_limit < 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_plan_groups_subscription_auto_approve_daily_limit_nonnegative'
    ) THEN
        ALTER TABLE plan_groups
            ADD CONSTRAINT ck_plan_groups_subscription_auto_approve_daily_limit_nonnegative
            CHECK (subscription_auto_approve_daily_limit >= 0);
    END IF;
END $$;

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS review_source VARCHAR(20);

CREATE INDEX IF NOT EXISTS idx_subscriptions_auto_quota_user_reviewed
    ON subscriptions (user_id, reviewed_at)
    WHERE review_source = 'AUTO_QUOTA';
