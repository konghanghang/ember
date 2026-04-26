-- failed_emby_async_ops：支付履约 / 兑换码 / 注册回滚等链路在事务外调 Emby 失败时
-- 的补偿队列。cron `@every 10m` 按 nextAttemptAt <= now() 拉取重试，成功删除该行；
-- 失败时指数退避（30s/2m/10m/1h/6h/24h）并写 lastError，retries > 6 写 ERROR 日志告警但保留行。
--
-- 字段说明：
--   origin       业务来源：payment_unban / redemption_unban / register_cleanup
--   originRefId  业务侧引用 ID：paymentId / redemptionId / embyUserId
--   embyUserId   待操作的 Emby 账号
--   action       unban / delete
--   payload      可选 JSON 文本（保留扩展字段，本批未使用）
--
-- 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS failed_emby_async_ops (
    id              varchar(25)  PRIMARY KEY,
    origin          varchar(32)  NOT NULL,
    "originRefId"   varchar(64)  NOT NULL,
    "embyUserId"    varchar(64)  NOT NULL,
    action          varchar(20)  NOT NULL,
    payload         text,
    retries         integer      NOT NULL DEFAULT 0,
    "nextAttemptAt" timestamptz  NOT NULL DEFAULT now(),
    "lastError"     varchar(500),
    "createdAt"     timestamptz  NOT NULL DEFAULT now(),
    "updatedAt"     timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_failed_emby_async_ops_next
    ON failed_emby_async_ops ("nextAttemptAt", retries);

CREATE INDEX IF NOT EXISTS idx_failed_emby_async_ops_origin
    ON failed_emby_async_ops (origin, "originRefId");
