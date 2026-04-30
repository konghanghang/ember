# 认证与账号完整性加固方案

> 状态：已归档（主干完成，转为历史追溯）
> 负责人：Ember
> 更新时间：2026-04-30

本文档已退出现行实施稿目录。当前认证 / 用户 / 邮件验证码完整性事实以 `docs/system-architecture.md` 与 `docs/reference/` 为准；本文件仅保留历史整改过程与决策追溯价值。

## 落地进度

批次 1（commits `333b926` + `043401a`）+ 后续 review 修复已完成本方案的 P0 / P1 主干项：

- ✅ 邮箱验证码发送限流按 `email+type` / `ip+type` advisory lock 收口，避免并发绕过
- ✅ 注册链路验证码消费并入注册事务；注册邀请码消费统一使用规范化后的 code
- ✅ 重置密码链路已补生产事务路径，验证码消费不再与主流程完全脱节
- ✅ 忘记密码反账号枚举（双重收口）：service 层 reset 路径无论邮箱是否注册都消耗同一套 IP / email 限流配额；handler 除 SMTP 未配置外所有错误一律折叠为 200 + 统一文案，攻击者无法借状态码或限流差异（200 vs 429）枚举注册邮箱
- ✅ 删除登录链路反向覆盖 Emby 密码的副作用
- ✅ EmbyID 错配显式拒登 + ERROR 日志
- ✅ `findLoginUser` / `ensureRegisterUserUnique` / `findUserByUsername` / `findUserByEmail` / SendVerificationCode 用户存在判断统一改 `lower(...)` 比较
- ✅ Schema 层补 `lower(username)` / `lower(email)` 函数唯一索引（`20260426_01_users_lower_unique_indexes.sql`，含预检 fail-fast 与排查 SQL），DB 兜底逻辑重复账号
- ✅ IP 限流 SQL 增加 `"type" = ?` 过滤；清理 `validateVerificationRateLimits` 之前的死分支与已无调用点的 `validateVerificationRecipient`
- ✅ `CheckExpiredUsers` 已补 `context cancel`、失败样本上限和 cron timeout
- ✅ `AuthService` / `UserService` / `EmailService` 已补显式依赖构造入口，运行期隐式 `setDefaults()` / `NewConfigService()` lazy 行为已收口
- ✅ ConfigService 敏感项已补 `maskedValue` 语义，设置中心可以稳定展示“已设置但不回显明文”的状态

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §5 已收录认证、用户、邮箱验证码与过期用户检查链路
- 当前规范：
  - [docs/reference/api-response-standard.md](</Users/konghang/data/me/github/ember/docs/reference/api-response-standard.md>)
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/me/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为已归档

## 退场说明

- 本文档不再承担当前认证 / 账号完整性规则说明职责；现行事实以 `docs/system-architecture.md` 与 `docs/reference/` 为准。
- 顶部状态、交叉引用与入口文档已完成归档收口，因此本文件只保留历史追溯价值。
