-- 20260427_04_bot_pending_reject_message_context
-- 用途：为 bot_pending_reject_requests 增加拒绝订阅消息上下文，避免 Bot 多实例下仍残留进程内状态依赖。
-- 变更：
--   - 新增 messageId / hasPhoto / originalText 三列
-- 幂等：是，可重复执行。

ALTER TABLE bot_pending_reject_requests
  ADD COLUMN IF NOT EXISTS "messageId" bigint,
  ADD COLUMN IF NOT EXISTS "hasPhoto" boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS "originalText" text NOT NULL DEFAULT '';
