-- 20260426_04_payments_checkout_constraints
-- 用途：统一收口 payments 的 checkout 并发约束与 stripe_session_id 唯一索引。
--
-- 改了哪些表、字段、索引、约束：
--   1. 为 `(user_id, plan_id) WHERE status='pending'` 增加 partial unique，
--      阻断双标签页连点结账造出多条 pending 订单。
--   2. 删除 baseline 中重复的 stripe_session_id 唯一索引，保留单一 partial unique
--      `idx_payments_stripe_session_id WHERE stripe_session_id <> ''`。
--
-- 是否需要回填：
--   - 需要预检并人工收口重复 pending 订单；存在脏数据时 migration 会 fail-fast 停止。
--
-- 是否可重复执行：是。DROP INDEX IF EXISTS / CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等。

DO $$
DECLARE
    dup_count int;
BEGIN
    SELECT count(*) INTO dup_count FROM (
        SELECT "user_id", "plan_id"
        FROM payments
        WHERE status = 'pending'
        GROUP BY "user_id", "plan_id"
        HAVING count(*) > 1
    ) t;

    IF dup_count > 0 THEN
        RAISE EXCEPTION
            'payments 存在 % 组同 (user_id, plan_id) 的多条 pending 订单；先收口再重跑：'
            'SELECT "user_id", "plan_id", count(*) FROM payments WHERE status=''pending'' GROUP BY 1,2 HAVING count(*) > 1; '
            '收口示例：UPDATE payments SET status=''expired'' WHERE id IN (SELECT id FROM payments p WHERE status=''pending'' AND id NOT IN (SELECT min(id) FROM payments WHERE status=''pending'' GROUP BY "user_id","plan_id"));',
            dup_count;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_pending_user_plan
    ON payments ("user_id", "plan_id")
    WHERE status = 'pending';

DROP INDEX IF EXISTS idx_payments_stripe_session;
DROP INDEX IF EXISTS idx_payments_stripe_session_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_stripe_session_id
    ON payments ("stripe_session_id")
    WHERE "stripe_session_id" <> '';
