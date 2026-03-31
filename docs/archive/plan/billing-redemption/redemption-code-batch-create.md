# 兑换码批量生成方案（已落地）

## 背景

现有管理端只能一次创建一个兑换码。运营要生成一批相同规则的兑换码时，只能重复提交同一张表单，纯属机械劳动。

这个问题不需要新模型。`redemption_codes` 本来就是一条码一条记录，所谓“批量生成”不过是一次事务里插入多条同规则记录。

## 目标

1. 管理员可一次生成多条随机兑换码
2. 批量生成不改变现有单个创建接口和行为
3. 批量结果可直接复制，不用回列表里逐个找
4. 保持现有兑换、注册、续期逻辑完全不变

## 设计

### 后端

保留现有接口：

- `POST /api/v1/admin/redemption-codes`

新增接口：

- `POST /api/v1/admin/redemption-codes/batch`

请求体：

```json
{
  "count": 20,
  "maxUses": 1,
  "defaultDays": 30,
  "expiresAt": "2026-03-31T23:59:59Z",
  "templateUserId": "ckxxxxxxxxxxxxxxxxxxxxxxx"
}
```

响应体：

```json
{
  "data": [
    {
      "id": "ckxxxxxxxxxxxxxxxxxxxxxxx",
      "code": "ab12cd34ef56ab78",
      "maxUses": 1,
      "usedCount": 0,
      "defaultDays": 30,
      "expiresAt": "2026-03-31T23:59:59Z",
      "createdAt": "2026-03-16T12:00:00Z"
    }
  ],
  "count": 1
}
```

约束：

1. `count` 必须在 `1..100`
2. 整批创建使用单事务，任一条失败则整批回滚
3. 码值继续使用随机 16 位 hex
4. 依赖 `redemption_codes.code` 唯一索引兜底碰撞；若命中唯一冲突，单条有限重试
5. 模板用户校验逻辑与单个创建一致

### 前端

入口仍为管理端兑换码页面 `services/web/src/views/admin/RedemptionCodesView.vue`。

创建弹窗新增：

1. `count` 数量输入框，默认 `1`
2. 数量为 `1` 时沿用现有单个创建接口
3. 数量大于 `1` 时调用批量接口

批量成功后：

1. 刷新兑换码列表
2. 打开结果弹窗
3. 展示本次生成的全部兑换码
4. 提供“一键复制全部”

## 与现有约束的关系

这次改动只影响“码的创建方式”，不影响“码的使用规则”。

以下行为保持不变：

1. 同一用户同一码最多成功一次
2. `usedCount` 仍按兑换成功次数递增
3. 过期码和用尽的码仍不可用
4. 模板用户权限仍只在邀请码注册时生效，续期兑换不受影响

## 验证

### 后端

```bash
cd services/api && go test ./...
cd services/api && go build ./...
```

关键场景：

1. `count=1` 和旧接口结果一致
2. `count>1` 返回数量正确且每条 `code` 唯一
3. 模板用户非法时返回 `400`
4. 数量超出 `1..100` 返回 `400`

### 前端

```bash
cd services/web && npm run build
```

关键场景：

1. 创建弹窗可输入数量
2. 数量为 `1` 时行为与旧版一致
3. 批量成功后可复制全部兑换码
4. 列表刷新后总数正确增加
