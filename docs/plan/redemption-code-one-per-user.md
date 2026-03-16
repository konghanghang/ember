# 兑换码“一人一码一次” + 兑换历史 UI 实现方案（已落地）

> 说明：本文档只覆盖“一人一码一次”与兑换历史相关改动。
> 兑换码批量生成请看 [redemption-code-batch-create.md](./redemption-code-batch-create.md)。
> 当前系统事实以 [docs/system-architecture.md](../system-architecture.md) 为准。

## Context

当前系统原始逻辑存在漏洞：同一用户可以重复兑换同一个兑换码，导致单个用户可能耗尽该码的全部 `maxUses`。

本次改造的目标：

1. 保证“同一用户 + 同一兑换码”最多成功一次
2. 保证同一码多人并发时不被慢外部依赖放大阻塞
3. 为用户端和管理端补齐兑换历史查询 UI

---

## 总体方案

### 后端约束策略（最终）

采用“应用层检查 + 数据库唯一约束兜底”的双保险：

1. 应用层在事务内先查 `redemptions(userId, code)`，命中则直接返回 `ErrRedemptionDuplicate`
2. 数据库层增加唯一索引 `UNIQUE ("userId", code)`，防止并发穿透
3. 插入 redemption 记录时捕获唯一冲突（PostgreSQL `23505`），统一映射为 `ErrRedemptionDuplicate`

### 并发与性能策略（最终）

1. 不再使用 `SELECT ... FOR UPDATE` 锁定 `redemption_codes` 行
2. 不在“同一码全局锁”路径上执行 Emby 外部调用
3. `usedCount` 仍使用条件原子更新，确保不会超过 `maxUses`

这样可以保持“同一码多人并发”可用性，同时满足“一人一码一次”的强约束。

---

## 1. 后端实现

### 1.1 核心逻辑：`RedeemCode`

**文件**：`services/api/internal/services/redemption.go`

最终流程：

1. 开启事务
2. 查询兑换码并校验有效性（不存在/失效直接返回）
3. 查询是否已存在 `(userId, code)` 兑换记录（已存在返回重复兑换）
4. 查询用户，计算新的到期时间
5. 若用户被封禁且激活中，则调用 Emby 解封，并更新 `embyDisabled=false`
6. 更新用户 `expiresAt`（和必要的 `embyDisabled`）
7. 插入 redemption 记录
   - 如命中唯一约束冲突（`23505`）=> 返回 `ErrRedemptionDuplicate`
8. 原子递增 `usedCount`：
   - 条件：`usedCount < maxUses` 且兑换码未过期
   - 若 `RowsAffected=0` => 返回 `ErrRedemptionCodeInvalid`
9. 提交事务

关键点：

1. “一人一码一次”最终由唯一索引保证
2. 去掉 `FOR UPDATE` 后，同一码高并发不再因慢 Emby 请求产生长队列
3. 仍保持 `usedCount` 不超限

### 1.2 错误定义与 Handler 映射

**文件**：`services/api/internal/services/redemption_errors.go`

新增错误：

```go
ErrRedemptionDuplicate = errors.New("你已经使用过此兑换码")
```

**文件**：

- `services/api/internal/handlers/user.go`
- `services/api/internal/handlers/telegram.go`

两处均将 `ErrRedemptionDuplicate` 映射为 HTTP 400。

---

## 2. 数据库迁移（必需）

> 与旧方案不同：本方案**必须**增加唯一索引。

### 2.1 迁移文件

新增文件：

- `infrastructure/database/20260304_01_add_redemptions_user_code_unique.sql`

迁移内容：

1. 清理 `redemptions` 历史重复记录（每组 `userId+code` 保留最早一条）
2. 创建唯一索引：`uq_redemptions_user_code` on `("userId", code)`
3. 回写 `redemption_codes.usedCount`，与 `redemptions` 实际行数对齐

### 2.2 为什么要先清理再建索引

如果表内已有重复数据，直接建唯一索引会失败。先清理历史脏数据，再加唯一索引，才能把规则固定在数据库层。

---

## 3. 前端实现

### 3.1 用户端 Dashboard 兑换历史

**文件**：`services/web/src/views/console/DashboardView.vue`

实现点：

1. 新增兑换历史区块（仅普通用户显示）
2. 接入 `/api/v1/user/redemptions` 分页查询
3. 兑换成功后并行刷新用户信息与兑换历史
4. 日期格式化抽到公共工具 `@/utils/date`

新增工具文件：

- `services/web/src/utils/date.ts`

### 3.2 管理端兑换历史页面

新增页面：

- `services/web/src/views/admin/RedemptionHistoryView.vue`

实现点：

1. 接入 `/api/v1/admin/redemptions`
2. 支持分页
3. 支持按 `userId` 筛选

### 3.3 路由与侧边栏

**文件**：

- `services/web/src/router/index.ts`
- `services/web/src/components/console/Sidebar.vue`

新增：

1. 管理员路由 `/console/redemption-history`
2. 管理端菜单“兑换历史”

---

## 4. 关键文件清单

### 后端

1. `services/api/internal/services/redemption.go`
2. `services/api/internal/services/errors.go`
3. `services/api/internal/handlers/user.go`
4. `services/api/internal/handlers/telegram.go`

### 数据库迁移

1. `infrastructure/database/20260304_01_add_redemptions_user_code_unique.sql`

### 前端

1. `services/web/src/views/console/DashboardView.vue`
2. `services/web/src/views/admin/RedemptionHistoryView.vue`
3. `services/web/src/router/index.ts`
4. `services/web/src/components/console/Sidebar.vue`
5. `services/web/src/utils/date.ts`

---

## 5. 验证方案

### 5.1 数据库验证

1. 无重复组：

```sql
SELECT "userId", code, COUNT(*)
FROM redemptions
GROUP BY "userId", code
HAVING COUNT(*) > 1;
```

2. 唯一索引存在：

```sql
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'redemptions'
  AND indexname = 'uq_redemptions_user_code';
```

### 5.2 后端验证

```bash
cd services/api && go build ./...
```

关键场景：

1. 同一用户同一码重复兑换：第一次成功，第二次返回 400 + `你已经使用过此兑换码`
2. 不同用户同一码并发兑换：不应因 Emby 慢调用导致同码全局长时间阻塞

### 5.3 前端验证

```bash
cd services/web && npm run build
```

关键场景：

1. 用户 Dashboard 可见兑换历史并可分页刷新
2. 管理端“兑换历史”菜单可进入并按 `userId` 筛选

---

## 6. 已知边界与取舍

1. Emby 调用仍在事务路径中（保持失败即回滚的语义），但已移除同码行锁，不再放大同码并发阻塞
2. 如果后续观察到事务时长仍对吞吐有影响，可再评估“提交后异步解封 Emby”方案
3. `usedCount` 与 `redemptions` 行数应长期一致，迁移已做一次性校准

---

## 7. 后续可选优化

1. 为运营报表补充“按兑换码去重用户数”的统计接口
2. 兑换历史支持导出 CSV
3. 若 `redemptions` 达到大规模（10w+），评估补充 `code` 单列索引用于报表查询
