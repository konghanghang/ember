-- payments：收口 baseline 中的重复 stripeSessionId 唯一索引，并改造为 partial unique
-- 以便支持 ON CONFLICT 风格的并发结账插入。
--
-- 背景：
--   1. baseline (20260415_00) 同时建立了 idx_payments_stripe_session 与
--      idx_payments_stripe_session_id，二者覆盖列完全相同，保留任一即可。
--   2. 批次 2 改造 CreateCheckoutSession：先按 (userId, planId) 唯一占位 pending 行
--      （此时 stripeSessionId 尚为空），再事务外调 Stripe 拿到真实 sessionID 回填。
--      若对 stripeSessionId 维持全表 UNIQUE NOT NULL DEFAULT ''，第二条占位行会因
--      空串重复而 INSERT 失败。改为 partial unique 排除空串后，多条占位行可共存，
--      真实 sessionID 仍保持全局唯一。
--
-- 幂等：DROP INDEX IF EXISTS + CREATE UNIQUE INDEX IF NOT EXISTS。

DROP INDEX IF EXISTS idx_payments_stripe_session;
DROP INDEX IF EXISTS idx_payments_stripe_session_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_stripe_session_id
    ON payments ("stripeSessionId")
    WHERE "stripeSessionId" <> '';
