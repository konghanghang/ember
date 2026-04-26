-- 20260428_10_subscriptions_note_not_null
-- 用途：将 subscriptions.note 列从可空改为 NOT NULL DEFAULT ''，
--       统一空备注语义，消灭 NULL 判断路径。
--
-- 改了哪些表、字段、索引、约束：
--   - subscriptions.note: NULL → NOT NULL DEFAULT ''
--
-- 是否需要回填：是，先将 NULL 回填为空字符串
-- 是否可重复执行：是（SET NOT NULL 对已是 NOT NULL 的列是 no-op；UPDATE 对无 NULL 的表是 no-op）

UPDATE subscriptions SET note = '' WHERE note IS NULL;
ALTER TABLE subscriptions ALTER COLUMN note SET NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN note SET DEFAULT '';
