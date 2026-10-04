-- 单分组套餐、用户独立期限和订单人工处理。
-- 本功能未发布，按用户确认原地修订；已执行旧稿的本地库须先处理组合商品，再同步本条 checksum。
-- 回填仅依据用户当前授权，不重放历史支付；永久由现有 NULL 期限语义映射。
-- 原人工封禁、账号状态和 Emby 同步状态不修改。等级由管理员配置，不猜测。
-- migration 外层事务保证原子性；事件幂等标识和列创建检查保证重跑不重发权益、不失效新码。
ALTER TABLE plan_groups ADD COLUMN IF NOT EXISTS entitlement_rank integer;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plan_groups_entitlement_rank ON plan_groups(entitlement_rank) WHERE entitlement_rank IS NOT NULL;
-- 商品只保存唯一分组与显式期限；订单快照保留数组，不受商品结构收敛影响。
ALTER TABLE plans ADD COLUMN IF NOT EXISTS validity_type varchar(20);
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'plans' AND column_name = 'benefits') THEN
        -- 旧本地测试稿可能存在组合商品，必须先明确其映射，不能自动截取第一项。
        IF EXISTS (SELECT 1 FROM plans WHERE benefits IS NOT NULL AND
            CASE WHEN jsonb_typeof(benefits) = 'array' THEN jsonb_array_length(benefits) <> 1 ELSE true END) THEN
            RAISE EXCEPTION 'single-group plans require explicit resolution of existing multi-benefit products';
        END IF;
        UPDATE plans SET
            plan_group = benefits->0->>'planGroup',
            validity_type = benefits->0->>'validityType',
            days = CASE WHEN benefits->0->>'validityType' = 'permanent' THEN 0
                        ELSE (benefits->0->>'durationDays')::integer END
        WHERE validity_type IS NULL AND benefits IS NOT NULL;
    END IF;
END $$;
-- 发布前旧基线只有正数天数，不能把异常 0 天自动当永久。
UPDATE plans SET validity_type = 'duration' WHERE validity_type IS NULL AND days > 0;
ALTER TABLE plans ALTER COLUMN validity_type SET NOT NULL;
ALTER TABLE plans ALTER COLUMN validity_type SET DEFAULT 'duration';
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'plans'::regclass AND conname = 'ck_plans_validity') THEN
        ALTER TABLE plans ADD CONSTRAINT ck_plans_validity CHECK (
            (validity_type = 'duration' AND days > 0) OR
            (validity_type = 'permanent' AND days = 0)
        );
    END IF;
END $$;
ALTER TABLE plans DROP COLUMN IF EXISTS benefits;
ALTER TABLE users ADD COLUMN IF NOT EXISTS resource_access_granted boolean;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS benefits jsonb;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS paid_at timestamptz;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS manual_review_reason varchar(100) NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS resolution varchar(30) NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS resolution_note varchar(500) NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS resolved_by varchar(100) NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS resolved_at timestamptz;

CREATE TABLE IF NOT EXISTS user_entitlements (
    user_id varchar(25) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_group varchar(50) NOT NULL REFERENCES plan_groups(key) ON DELETE RESTRICT,
    validity_type varchar(20) NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, plan_group),
    CONSTRAINT ck_user_entitlements_validity CHECK (
        (validity_type = 'permanent' AND expires_at IS NULL) OR
        (validity_type = 'duration' AND expires_at IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_user_entitlements_expires_at ON user_entitlements(expires_at) WHERE expires_at IS NOT NULL;
CREATE TABLE IF NOT EXISTS entitlement_events (
    source_key varchar(200) PRIMARY KEY,
    user_id varchar(25) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor varchar(100) NOT NULL,
    reason varchar(100) NOT NULL,
    before_state jsonb NOT NULL,
    after_state jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_entitlement_events_user_id ON entitlement_events(user_id);

-- 无法解析原分组时阻止迁移，不悄悄丢失或扩大权限。
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM users u LEFT JOIN plan_groups g
        ON g.key = COALESCE(u.plan_group, (SELECT key FROM plan_groups WHERE is_default))
        WHERE u.role = 'user' AND u.resource_access_granted IS NULL AND g.key IS NULL
    ) THEN RAISE EXCEPTION 'entitlement migration requires valid existing user groups'; END IF;
END $$;

WITH source AS (
    SELECT u.id, COALESCE(u.plan_group, (SELECT key FROM plan_groups WHERE is_default)) AS group_key,
           u.expires_at, CASE WHEN u.expires_at IS NULL THEN 'permanent' ELSE 'duration' END AS validity
    FROM users u WHERE u.role = 'user' AND u.resource_access_granted IS NULL
), marked AS (
    INSERT INTO entitlement_events(source_key, user_id, actor, reason, before_state, after_state)
    SELECT 'migration:entitlements:' || id, id, 'system:migration', 'legacy_user', '[]'::jsonb,
           jsonb_build_array(jsonb_build_object('planGroup', group_key, 'validityType', validity, 'expiresAt', expires_at))
    FROM source ON CONFLICT (source_key) DO NOTHING RETURNING user_id
), granted AS (
    INSERT INTO user_entitlements(user_id, plan_group, validity_type, expires_at)
    SELECT s.id, s.group_key, s.validity, s.expires_at FROM source s JOIN marked m ON m.user_id = s.id
    ON CONFLICT (user_id, plan_group) DO NOTHING RETURNING user_id
)
UPDATE users u SET plan_group = s.group_key,
    resource_access_granted = (s.expires_at IS NULL OR s.expires_at > now())
FROM source s JOIN marked m ON m.user_id = s.id WHERE u.id = s.id;

UPDATE users SET resource_access_granted = true WHERE role = 'admin' AND resource_access_granted IS NULL;
ALTER TABLE users ALTER COLUMN resource_access_granted SET DEFAULT false;
ALTER TABLE users ALTER COLUMN resource_access_granted SET NOT NULL;

-- 仅首次新增列时让所有旧码失效；重跑不触碰新码，兑换历史不删除。
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema()
        AND table_name = 'redemption_codes' AND column_name = 'legacy_invalidated') THEN
        ALTER TABLE redemption_codes ADD COLUMN legacy_invalidated boolean NOT NULL DEFAULT true;
        ALTER TABLE redemption_codes ALTER COLUMN legacy_invalidated SET DEFAULT false;
    END IF;
END $$;
