#!/usr/bin/env node
/**
 * Ember - 创建管理员账号工具
 *
 * 用途：生成带有自定义密码的管理员账号 SQL
 *
 * 使用方法：
 *   node scripts/create-admin.js [用户名] [密码]
 *
 * 示例：
 *   node scripts/create-admin.js admin MySecurePassword123
 *   node scripts/create-admin.js  # 使用默认账号 admin/admin123
 */

const bcrypt = require('bcryptjs');
const { randomBytes } = require('crypto');

// 生成 CUID 风格的 ID
function generateId() {
  const timestamp = Date.now().toString(36);
  const random = randomBytes(8).toString('base64').replace(/[^a-z0-9]/gi, '').toLowerCase().substring(0, 16);
  return `cm3${timestamp}${random}`.substring(0, 25).padEnd(25, '0');
}

// 获取命令行参数
const [,, username = 'admin', password = 'admin123'] = process.argv;

// 生成 bcrypt hash (cost=10)
const passwordHash = bcrypt.hashSync(password, 10);
const userId = generateId();

// 生成 SQL
const sql = `-- ================================================
-- Ember - 管理员账号初始化
-- ================================================
--
-- 账号信息：
--   用户名：${username}
--   密码：${password}
--
-- ⚠️ 请妥善保管此文件，或在执行后删除���
-- ================================================

-- 插入管理员账号
INSERT INTO admins (id, username, password, "createdAt", "updatedAt")
VALUES (
    '${userId}',
    '${username}',
    '${passwordHash}',
    NOW(),
    NOW()
)
ON CONFLICT (username) DO UPDATE SET
    password = EXCLUDED.password,
    "updatedAt" = NOW();

-- 验证插入结果
SELECT
    id,
    username,
    "createdAt",
    "updatedAt"
FROM admins
WHERE username = '${username}';
`;

console.log(sql);
console.log('-- ================================================');
console.log(`-- ✅ SQL 已生成！保存到文件或直接执行：`);
console.log(`--`);
console.log(`-- 方式 1：保存并执行`);
console.log(`--   node scripts/create-admin.js ${username} ${password} > /tmp/create-admin.sql`);
console.log(`--   psql $DATABASE_URL -f /tmp/create-admin.sql`);
console.log(`--`);
console.log(`-- 方式 2：直接执行`);
console.log(`--   node scripts/create-admin.js ${username} ${password} | psql $DATABASE_URL`);
console.log('-- ================================================');
