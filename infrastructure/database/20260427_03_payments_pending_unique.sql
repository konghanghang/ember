-- payments：同 (userId, planId) status='pending' partial unique，
-- 阻断双标签页连点结账触发 Stripe 重复 Session 与重复扣费。
--
-- 业务侧 CreateCheckoutSession 配套改造：先 INSERT ... ON CONFLICT DO NOTHING，
-- 命中冲突回查现有 pending 复用 checkoutUrl。详见
-- services/api/internal/services/payment/service.go。
--
-- 历史脏数据预检：直接 CREATE UNIQUE INDEX 在已有重复 pending 时会失败，并连带
-- API 启动期 VerifySchema 报错。本 migration 用 DO $$ ... $$ 块在建索引前显式预检；
-- 若发现冲突立即 RAISE EXCEPTION，错误信息附上排查 + 收口 SQL，运维需先把多余
-- pending 收口为 expired 再重跑。
--
-- 幂等：CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等；预检在已建好索引的环境上为
-- 0 行结果，DO 块直接通过。

DO $$
DECLARE
    dup_count int;
BEGIN
    SELECT count(*) INTO dup_count FROM (
        SELECT "userId", "planId"
        FROM payments
        WHERE status = 'pending'
        GROUP BY "userId", "planId"
        HAVING count(*) > 1
    ) t;

    IF dup_count > 0 THEN
        RAISE EXCEPTION
            'payments 存在 % 组同 (userId, planId) 的多条 pending 订单；先收口再重跑：'
            'SELECT "userId", "planId", count(*) FROM payments WHERE status=''pending'' GROUP BY 1,2 HAVING count(*) > 1; '
            '收口示例：UPDATE payments SET status=''expired'' WHERE id IN (SELECT id FROM payments p WHERE status=''pending'' AND id NOT IN (SELECT min(id) FROM payments WHERE status=''pending'' GROUP BY "userId","planId"));',
            dup_count;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_pending_user_plan
    ON payments ("userId", "planId")
    WHERE status = 'pending';
