-- 一个用户同一时刻只允许一条 telegram_bind_codes，确保 GenerateBindCode 的 ON CONFLICT(userId) 语义可靠。
--
-- 老库由 baseline 创建了非唯一 idx_telegram_bind_codes_user_id（按 userId 索引），
-- 本 migration 在同一列上额外创建唯一索引，提供原子互斥保证；不删除原非唯一索引。
--
-- 幂等：CREATE UNIQUE INDEX IF NOT EXISTS。
-- 前置：本 migration 假设上线前已不存在同 userId 的多条记录；如有需要先做去重。

CREATE UNIQUE INDEX IF NOT EXISTS uq_telegram_bind_codes_user
  ON telegram_bind_codes USING btree ("userId");
