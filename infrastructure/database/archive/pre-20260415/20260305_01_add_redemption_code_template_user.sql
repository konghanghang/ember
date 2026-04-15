-- Ember migration: 为 redemption_codes 增加模板用户字段
-- Date: 2026-03-05
--
-- Purpose:
-- 1) 新增 redemption_codes.templateUserId（可空）
-- 2) 为 templateUserId 增加索引，优化模板用户筛选与关联查询
--
-- Notes:
-- - 脚本幂等，可重复执行。
-- - 保持与 GORM 列名一致：camelCase 需使用双引号。

BEGIN;

ALTER TABLE redemption_codes
  ADD COLUMN IF NOT EXISTS "templateUserId" varchar(25);

CREATE INDEX IF NOT EXISTS idx_redemption_codes_template_user_id
  ON redemption_codes ("templateUserId");

COMMIT;
