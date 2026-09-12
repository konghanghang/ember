-- 分离账号控制面配置代次与运行期健康 updated_at；无新索引或约束变更。
-- 已有行通过常量 DEFAULT 初始化为 1，无需业务回填；重复执行保留已有代次。
ALTER TABLE p115_accounts
    ADD COLUMN IF NOT EXISTS config_version BIGINT NOT NULL DEFAULT 1;

COMMENT ON COLUMN p115_accounts.config_version IS
    '控制面配置与凭证代次；健康回写和冷却探测不递增';
