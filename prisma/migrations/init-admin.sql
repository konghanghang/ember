-- ================================================
-- Ember - 初始化管理员账号
-- ================================================
--
-- 用途：创建默认管理员账号，用于首次登录
--
-- 默认账号：
--   用户名：admin
--   密码：admin123
--
-- ⚠️ 安全提醒：
--   1. 首次登录后立即修改密码！
--   2. 生产环境建议使用强密码
--   3. 此脚本可重复执行（使用 ON CONFLICT 避免重复插入）
--
-- 执行方式：
--   psql $DATABASE_URL -f prisma/migrations/init-admin.sql
-- ================================================

-- 插入默认管理员账号
-- 密码：admin123 (bcrypt hash, cost=10)
INSERT INTO admins (id, username, password, "createdAt", "updatedAt")
VALUES (
    'cm3admin00000000000000000',  -- 固定 ID（方便识别）
    'admin',
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',  -- admin123
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;  -- 如果已存在则跳过

-- 验证插入结果
SELECT
    id,
    username,
    "createdAt"
FROM admins
WHERE username = 'admin';

-- 输出提示信息
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM admins WHERE username = 'admin') THEN
        RAISE NOTICE '✅ 管理员账号初始化成功';
        RAISE NOTICE '   用户名: admin';
        RAISE NOTICE '   密码: admin123';
        RAISE NOTICE '   ⚠️  请登录后立即修改密码！';
    ELSE
        RAISE WARNING '❌ 管理员账号初始化失败';
    END IF;
END $$;
