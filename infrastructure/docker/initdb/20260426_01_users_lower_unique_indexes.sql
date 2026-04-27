-- 用户名 / 邮箱大小写不敏感唯一约束。
--
-- 批次 1 把登录与注册唯一性校验改为 lower(username) / lower(email) 比较，但若数据库未补对应
-- 唯一索引，并发注册或后台创建仍会写入大小写不同的逻辑重复账号；新登录链路按 createdAt ASC
-- 取最早一条，会让后注册的同名大小写账号永远登不到自己。本 migration 把唯一性约束下沉到 DB。
--
-- 历史脏数据处理：直接 CREATE UNIQUE INDEX 在已有重复时会失败，并连带 API 启动期 VerifySchema
-- 报错。本 migration 用 DO $$ ... $$ 块在建索引前显式预检；若发现冲突立即 RAISE EXCEPTION，
-- 错误信息附上排查 SQL，运维需先去重再重跑。
--
-- 幂等：CREATE UNIQUE INDEX IF NOT EXISTS 自身幂等；预检在已建好索引的环境上为 0 行结果，DO 块直接通过。

DO $$
DECLARE
    dup_username_count int;
    dup_email_count int;
BEGIN
    SELECT count(*) INTO dup_username_count FROM (
        SELECT lower(username) FROM users GROUP BY lower(username) HAVING count(*) > 1
    ) t;

    SELECT count(*) INTO dup_email_count FROM (
        SELECT lower(email)
        FROM users
        WHERE email IS NOT NULL AND email <> ''
        GROUP BY lower(email)
        HAVING count(*) > 1
    ) t;

    IF dup_username_count > 0 OR dup_email_count > 0 THEN
        RAISE EXCEPTION
            'users 表存在大小写不敏感重复 (username=%, email=%); 排查并去重：'
            'SELECT lower(username), count(*) FROM users GROUP BY 1 HAVING count(*) > 1; '
            'SELECT lower(email), count(*) FROM users WHERE email IS NOT NULL AND email <> '''' GROUP BY 1 HAVING count(*) > 1;',
            dup_username_count,
            dup_email_count;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_username_lower
  ON users USING btree (lower(username));

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower
  ON users USING btree (lower(email))
  WHERE email IS NOT NULL AND email <> '';
