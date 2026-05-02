-- 20260427_04_bot_pending_reject_message_context
-- 用途：为 bot_pending_reject_requests 增加拒绝订阅消息上下文，避免 Bot 重启或滚动发布导致待输入状态丢失。
-- 变更：
--   - 新增 message_id / has_photo / original_text 三列
-- 幂等：是，可重复执行。

ALTER TABLE bot_pending_reject_requests
  ADD COLUMN IF NOT EXISTS "message_id" bigint,
  ADD COLUMN IF NOT EXISTS "has_photo" boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS "original_text" text NOT NULL DEFAULT '';
