-- bot_runtime_locks：Bot polling 模式单实例租约锁。
--
-- 用途：
--   - Bot 在 TELEGRAM_UPDATE_MODE=polling 时通过 Internal API 申请/续租/释放租约
--   - 多实例竞争时只有一份 ownerId 能持有 telegram_polling 锁
--   - owner 崩溃不释放时，租约到期后允许下一实例接管
--
-- 幂等：CREATE TABLE/INDEX IF NOT EXISTS

CREATE TABLE IF NOT EXISTS bot_runtime_locks (
  name        varchar(100) PRIMARY KEY,
  "ownerId"   varchar(200) NOT NULL,
  "expiresAt" timestamptz  NOT NULL,
  "createdAt" timestamptz  NOT NULL DEFAULT now(),
  "updatedAt" timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bot_runtime_locks_expires
  ON bot_runtime_locks ("expiresAt");
