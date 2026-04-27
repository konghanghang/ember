-- 20260426_15_users_password_reset_required
-- 用途：为 users 表新增 passwordResetRequired 列，用于标记首次登录必须修改密码。
--
-- 改了哪些表、字段、索引、约束：
--   - users 表新增 passwordResetRequired boolean 列（NOT NULL DEFAULT false）
--
-- 是否需要回填：否（DEFAULT false 对所有已有用户生效）
-- 是否可重复执行：是（ADD COLUMN IF NOT EXISTS）

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS "passwordResetRequired" boolean NOT NULL DEFAULT false;
