-- users.emby_id 偏唯一约束（管理员 Emby 账号绑定能力配套）。
--
-- 表 / 字段 / 索引：
--   表：users
--   字段：emby_id（不变更字段定义）
--   旧索引：idx_users_emby_id（普通 btree 索引，不能阻止两个本地账号绑同一个 Emby 用户）
--   新索引：uniq_users_emby_id（PostgreSQL 偏唯一索引，emby_id 非空且非空串时唯一）
--
-- 改动语义：
--   管理员账号现在可以通过控制台自助把当前账号关联到一个真实 Emby 用户。应用层会校验
--   占用情况，但需要 DB 层兜底并发：两个管理员同时绑定到同一个 Emby 用户时，必须由
--   唯一约束硬阻止，避免脏数据；同时未绑定（emby_id = '' 或 NULL）的多条记录不受
--   该约束限制，普通用户在登录前不会被影响。
--
-- 是否需要回填：不需要。已确认现网无 emby_id 重复行；普通用户由注册链路写入 emby_id，
--   历史均为一对一映射。
--
-- 是否可重复执行：是。DROP INDEX IF EXISTS + CREATE UNIQUE INDEX IF NOT EXISTS，整文件幂等。
--
-- 命名约定：本仓库已收口 snake_case；新唯一索引使用 uniq_users_emby_id，与现有 uq_*
--   命名虽略有差别但在 baseline 与历史增量内同时存在，遵循"按改动语义最贴近的命名"原则。

DROP INDEX IF EXISTS idx_users_emby_id;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_users_emby_id
    ON users (emby_id)
    WHERE emby_id IS NOT NULL AND emby_id <> '';
