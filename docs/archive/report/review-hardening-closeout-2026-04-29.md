# Review 硬化收尾记录（2026-04-29）

## 背景

2026-04-27 起，本仓库围绕主干 8 份计划执行了一轮系统性 review 修复，目标不是继续扩功能，而是把已经落地的主链路风险集中收口。

本记录只保留三类信息：

1. 本轮主要修了什么
2. 对应提交是什么
3. 还剩哪些明确尾项

它是历史追溯材料，不承担现行规范说明；当前事实以 `docs/system-architecture.md` 和各计划文档顶部状态为准。

## 已收口问题

### 认证 / 支付 / 兑换

- 支付过期单与旧 Stripe Checkout Session 失效收口
- 邮箱验证码发送限流并发绕过
- `VerifySchema` 对支付索引迁移漏检
- `InternalAuth` 常数时间比较与运行期读取
- 默认管理员强制改密闭环
- 注册邀请码 `trim` 与真实消费不一致
- 注册 / 重置密码验证码事务路径

### 订阅 / 缺集 / TV Calendar

- `media_gaps` 扫描整行回写导致状态机回退
- `IGNORED` 工单被扫描或 webhook 自动复活
- TV Calendar 当前周纠偏只改响应不改库
- TV Calendar webhook `seriesId` 宽匹配误命中
- 整剧进度统计把系统忽略当成人工排除
- 订阅终态 `mpError` 被异步污染
- 缺集扫描启动失败 / 锁冲突返回语义

### Bot / 部署 / 前端

- Bot runtime settings 失败覆盖旧值
- 拒绝订阅持久化缺口
- Bot `polling` 单实例缺少硬保护
- 部署入口应用镜像使用 floating `latest`
- 跨标签登录态切换后的路由收口
- 设备黑名单批量注销新返回结构消费
- 高价值前端请求竞态（续费、订阅、最近入库、设备、画像、播放历史）
- `useUserStore.subscriptions` 双轨状态残留

## 提交记录

按时间顺序，本轮相关提交如下：

1. `869242f` `fix(core): 收口 review 第一批高优先级问题`
2. `c33fa67` `fix(review): 收口第二批状态机与画像链路问题`
3. `2830214` `fix(review): 收口第三批契约与事务边界问题`
4. `d583c0c` `fix(web): 收口请求竞态并固定部署镜像基线`
5. `12684d6` `fix(bot): 为 polling 模式补单实例租约锁`
6. `4ae05da` `refactor(web): 清理双轨状态并同步计划事实`

## 验证

本轮反复执行并通过的验证包括：

- `cd services/api && go test ./...`
- `cd services/web && npm run build`
- `PYTHONPYCACHEPREFIX=/tmp/ember-pycache python3 -m py_compile services/bot/app/clients/api_client.py services/bot/app/server.py`

## 当前剩余尾项

截至 2026-04-29，主干硬问题已基本收口，剩余项主要是治理尾项：

- BotNotifier 配置缓存、通知载荷长度与 `message_id` 缓存策略
- TV Calendar 三层缓存、`pickTargetSeasonNumbers`、`tmdb_cache` GC
- `CheckExpiredUsers` cancel / 失败上限 / 更深层 DI 治理
- 更广范围的前端请求竞态 sweep
- 部署 runbook 细化、baseline 精简归档、文档盘点持续同步

## 归档判断

这轮修复本身已完成，但主干 8 份计划仍未全部满足“状态、验证、稳定结论提炼、交叉引用同步”四项归档条件，因此当前只归档本记录，不归档主计划正文。
