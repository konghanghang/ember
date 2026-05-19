-- 20260519_01_subscription_admin_notifications
-- 用途：持久化订阅管理员 Telegram 审批消息投递引用，支持多管理员消息同步更新。
-- 变更：
--   - 新增 subscription_admin_notifications 表
--   - 新增按 subscription_id 查询、按 admin_telegram_id 排查和按消息引用去重的索引
--   - 新增设置项 telegram_approval_admin_ids，显式保存 Telegram 审批人员 user_id 列表
-- 回填：历史未持久化的 Telegram 消息无法回填，本迁移只影响新投递消息。
-- 幂等：是，可重复执行。

CREATE TABLE IF NOT EXISTS subscription_admin_notifications (
  id varchar(25) PRIMARY KEY,
  subscription_id varchar(25) NOT NULL,
  admin_telegram_id bigint NOT NULL,
  chat_id bigint NOT NULL,
  message_id bigint,
  has_photo boolean NOT NULL DEFAULT false,
  delivery_status varchar(20) NOT NULL DEFAULT 'sent',
  failure_reason varchar(500),
  created_at timestamptz DEFAULT now(),
  updated_at timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscription_admin_notifications_subscription
  ON subscription_admin_notifications (subscription_id);

CREATE INDEX IF NOT EXISTS idx_subscription_admin_notifications_admin
  ON subscription_admin_notifications (admin_telegram_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_subscription_admin_notifications_message
  ON subscription_admin_notifications (chat_id, message_id)
  WHERE message_id IS NOT NULL;

INSERT INTO settings (key, value, updated_at, is_encrypted, updated_by_user_id)
VALUES ('telegram_approval_admin_ids', '', now(), false, NULL)
ON CONFLICT (key) DO NOTHING;
