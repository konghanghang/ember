-- 20260426_15_users_password_reset_required
-- 用途：为 users 表新增 password_reset_required 列，用于标记首次登录必须修改密码。
--
-- 改了哪些表、字段、索引、约束：
--   - users 表新增 password_reset_required boolean 列（NOT NULL DEFAULT false）
--
-- 是否需要回填：否（DEFAULT false 对所有已有用户生效）
-- 是否可重复执行：是（ADD COLUMN IF NOT EXISTS）

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS "password_reset_required" boolean NOT NULL DEFAULT false;
