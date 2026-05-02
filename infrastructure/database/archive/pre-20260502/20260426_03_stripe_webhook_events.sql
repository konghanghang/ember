-- stripe_webhook_events：Stripe webhook event.id 级别去重表。
--
-- Stripe Dashboard "Resend" / 网络抖动重投会推送同 event.id 多次；如果只靠业务侧
-- 状态机判重，跨事件类型（completed + async_payment_succeeded 同 sessionID）会无法
-- 区分；这里以 event.id 为主键，第一次 INSERT 成功才进入业务分发，后续命中 ON CONFLICT
-- 直接 200。处理完成后单事务把 status / processed_at 收口。
--
-- 字段说明：
--   event_id      Stripe event.id（evt_xxx），主键
--   event_type    Stripe event.type（checkout.session.completed 等）
--   livemode     Stripe event.livemode
--   received_at   首次收到时间
--   processed_at  业务分发完成时间
--   status       received / processed / skipped / failed
--   error_message 业务分发失败时的脱敏错误（不写完整 wrap 链）
--
-- 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS stripe_webhook_events (
    "event_id"      varchar(64)  PRIMARY KEY,
    "event_type"    varchar(64)  NOT NULL,
    livemode       boolean      NOT NULL DEFAULT false,
    "received_at"   timestamptz  NOT NULL DEFAULT now(),
    "processed_at"  timestamptz,
    status         varchar(20)  NOT NULL DEFAULT 'received',
    "error_message" varchar(500)
);

CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_received
    ON stripe_webhook_events ("received_at");
