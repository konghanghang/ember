-- 一个用户同一时刻只允许一条 telegram_bind_codes，确保 GenerateBindCode 的 ON CONFLICT(user_id) 语义可靠。
--
-- 老库由 baseline 创建了非唯一 idx_telegram_bind_codes_user_id（按 user_id 索引），
-- 本 migration 在同一列上额外创建唯一索引，提供原子互斥保证；不删除原非唯一索引。
--
-- 历史脏数据处理：旧 GenerateBindCode 是事务里 DELETE+INSERT，在并发下可能留下同一 user_id
-- 的多条绑定码；直接 CREATE UNIQUE INDEX 会因冲突失败、API 启动期 VerifySchema 也会停摆。
-- 本 migration 在建索引前显式去重：每个 user_id 只保留 created_at 最新的一条。
-- 绑定码本就是短期凭据（5 分钟有效），删除冗余历史行不会影响业务。
--
-- 幂等：去重的 DELETE 在二次执行时 row_number() = 1 永远成立 → 0 行受影响；
--       CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等。

WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY "user_id" ORDER BY "created_at" DESC, id DESC) AS rn
    FROM telegram_bind_codes
)
DELETE FROM telegram_bind_codes
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX IF NOT EXISTS uq_telegram_bind_codes_user
  ON telegram_bind_codes USING btree ("user_id");
