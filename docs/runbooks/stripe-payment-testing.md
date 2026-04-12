# Stripe 支付测试指南

这份手册只回答一件事：在 Ember 里怎么把 Stripe 支付链路测通。

目标链路：

1. 用户在续费中心发起购买
2. API 创建 Stripe Checkout Session
3. 用户在 Stripe 测试页完成或取消支付
4. Stripe Webhook 回调 Ember API
5. Ember 更新 `payments` 状态并延长 `users.expiresAt`

不管你用 `Stripe CLI` 还是 `cloudflared`，真正算测通的标准都一样：**Webhook 到达并完成履约**。前端跳回 `success=true` 只能说明跳转成功，不能代替后端履约。

---

## 1. 适用场景

- 你要验证 Stripe 支付功能本身
- 你要联调 Checkout Session 创建、Webhook 验签和支付履约
- 你改了支付配置、支付方式限制、套餐分组、支付记录或续期逻辑

不适用：

- 只想看前端页面是否能跳转 Stripe
- 只想验证某个测试卡号是否能在 Stripe 页面通过

---

## 2. 当前实现要点

先记住这几个事实，别一边测一边猜：

- Checkout 创建入口：`POST /api/v1/payments/checkout`
- Webhook 入口：`POST /api/v1/webhooks/stripe`
- 用户页入口：`/console/renewal`
- 成功页和取消页由 `STRIPE_SUCCESS_URL`、`STRIPE_CANCEL_URL` 控制
- 履约依赖 Webhook，不依赖前端成功页
- 同一用户、同一方案、30 分钟内未过期的待支付订单会被复用

如果你重复点购买却发现还是旧链接，不是 Stripe 抽风，是 Ember 的正常行为。

---

## 3. 前置条件

### 3.1 必要配置

先补齐以下配置：

| 配置项 | 来源 | 说明 |
|--------|------|------|
| `STRIPE_SECRET_KEY` | 设置中心 | Stripe 服务端测试密钥，建议使用 `sk_test_...` |
| `STRIPE_SUCCESS_URL` | 设置中心 | 支付成功跳转地址 |
| `STRIPE_CANCEL_URL` | 设置中心 | 支付取消跳转地址 |
| `STRIPE_WEBHOOK_SECRET` | API 环境变量 | Stripe Webhook 签名密钥，只能走环境变量 |

推荐值：

```text
STRIPE_SUCCESS_URL=http://localhost:3000/console/renewal?success=true
STRIPE_CANCEL_URL=http://localhost:3000/console/renewal?canceled=true
```

如果你的前端不是跑在 `3000`，把域名和端口换成你的实际地址。

### 3.2 套餐与用户准备

- 后台先创建至少一个启用中的套餐
- 测试用户必须能看到这个套餐
- 如果当前用户有套餐分组，套餐也必须属于同一有效分组

### 3.3 Stripe Dashboard 准备

- 切到 Stripe `test mode`
- 如果要测动态支付方式，先在 Stripe Dashboard 开启对应支付方式
- 如果只是先测主链路，建议把系统里的 `stripe_allowed_payment_methods` 限制成：

```json
["card"]
```

这样最少变量，先把信用卡主路径跑通。

---

## 4. 成功标准

一次成功的测试至少要满足这几条：

- 用户能从 Ember 跳转到 Stripe Checkout
- Stripe 支付完成后，Webhook 成功打到 `POST /api/v1/webhooks/stripe`
- 对应 `payments.status` 变成 `completed`
- 对应 `users.expiresAt` 被正确顺延
- 已过期且被禁用的 Emby 用户，支付成功后自动恢复

如果只看到前端跳回 `?success=true`，但 `payments` 还是 `pending`，这次测试就是失败。

---

## 5. 方式一：使用 Stripe CLI

这是本地测试最省事的方案。

### 5.1 安装并登录

安装完成后执行：

```bash
stripe login
```

通常会跳浏览器授权；如果没有自动跳，终端里会给一个 URL，手动复制到浏览器打开即可。

### 5.2 启动 Webhook 转发

```bash
stripe listen \
  --events checkout.session.completed,checkout.session.async_payment_succeeded,checkout.session.async_payment_failed \
  --forward-to http://localhost:8080/api/v1/webhooks/stripe
```

命令启动后，Stripe CLI 会输出一个新的 `whsec_...`。

### 5.3 配置 Webhook Secret

- 把 Stripe CLI 输出的 `whsec_...` 写入 API 进程环境变量 `STRIPE_WEBHOOK_SECRET`
- 重启 API，让新环境变量生效

### 5.4 发起真实支付

1. 登录 Ember 测试用户
2. 打开 `/console/renewal`
3. 选择一个方案并点击购买
4. 页面跳转到 Stripe Checkout

### 5.5 测试卡

先用最基础的成功卡：

```text
4242 4242 4242 4242
```

其他字段：

- 到期日：任意未来日期
- CVC：任意 3 位
- 邮编：任意合法值

### 5.6 验证结果

支付完成后检查：

- 前端是否跳回 `?success=true`
- API 日志是否出现收到 Stripe Webhook
- API 日志是否出现支付履约成功
- 用户支付记录里该订单是否变成 `completed`
- 用户到期时间是否增加

### 5.7 失败与取消验证

取消支付：

- 在 Stripe Checkout 页面点击取消
- 预期前端跳回 `?canceled=true`
- 预期不发生续期

拒付卡：

```text
4000 0000 0000 0002
```

预期：

- Stripe 页面提示失败
- Ember 不应错误续期

---

## 6. 方式二：使用 cloudflared

这条路不依赖 Stripe CLI，但你必须给 Stripe 一个公网 HTTPS 回调地址。

### 6.1 适用场景

- 不想安装 Stripe CLI
- 本地要保持真实公网 Webhook 入口
- 想让 Stripe 直接打到你的本地 API

### 6.2 启动隧道

本地 API 假设跑在 `8080`：

```bash
cloudflared tunnel --protocol http2 --edge-ip-version 4 --url http://localhost:8080
```

命令输出里会给一个公网地址，例如：

```text
https://xxxx.trycloudflare.com
```

### 6.3 在 Stripe Dashboard / Workbench 配置 Webhook Endpoint

把以下地址注册为 Webhook Endpoint：

```text
https://xxxx.trycloudflare.com/api/v1/webhooks/stripe
```

建议订阅以下事件：

- `checkout.session.completed`
- `checkout.session.async_payment_succeeded`
- `checkout.session.async_payment_failed`

### 6.4 配置 Webhook Secret

- Stripe 会为这个 endpoint 生成一个 `whsec_...`
- 把它写到 API 进程环境变量 `STRIPE_WEBHOOK_SECRET`
- 重启 API

### 6.5 发起支付与验证

后续步骤和 Stripe CLI 方案完全一样：

1. 进入 `/console/renewal`
2. 选择方案
3. 跳转 Stripe Checkout
4. 用测试卡支付
5. 检查前端回跳、Webhook 日志、支付记录和到期时间

---

## 7. 推荐测试顺序

不要一上来就把所有支付方式一起测。按这个顺序做，最省时间：

### 7.1 第一轮：信用卡主链路

- `stripe_allowed_payment_methods = ["card"]`
- 跑一次成功支付
- 跑一次取消支付
- 跑一次拒付卡

### 7.2 第二轮：异步支付方式

放开：

```json
["card","alipay"]
```

或：

```json
["card","wechat_pay"]
```

重点验证：

- `checkout.session.completed` 但 `payment_status != paid` 时，不应提前履约
- 只有在 `checkout.session.async_payment_succeeded` 后才真正续期
- `checkout.session.async_payment_failed` 时应标记失败，不应续期

---

## 8. 每次测试都要看的检查项

### 8.1 前端

- `/console/renewal` 方案列表是否正常展示
- 点击购买后是否跳转 Stripe
- `?success=true` / `?canceled=true` 提示是否符合预期

### 8.2 API

- `POST /api/v1/payments/checkout` 是否返回 URL
- `POST /api/v1/webhooks/stripe` 是否收到请求
- Webhook 验签是否通过

### 8.3 数据

- `payments.stripeSessionId`
- `payments.stripePaymentIntentId`
- `payments.status`
- `users.expiresAt`

### 8.4 外部副作用

- 已过期 Emby 用户是否自动恢复
- 如果配置了 Bot 通知，管理员是否收到支付成功通知

---

## 9. 常见问题

### 9.1 看到 `success=true`，但没有续期

原因通常只有三个：

- `STRIPE_WEBHOOK_SECRET` 配错了
- Webhook 根本没有打到本地 API
- Webhook 打到了，但事件对应不到本地 `payments.stripeSessionId`

先查 API 日志，不要先怀疑前端。

### 9.2 重复点购买，为什么还是老的 Stripe 页面

这是正常行为。Ember 会复用同一用户、同一方案、30 分钟内未过期的待支付订单。

处理方式：

- 换一个方案测试
- 等旧订单过期
- 或者先清理测试数据再测

### 9.3 测支付宝或微信支付时，为什么成功页已经回来了，但权益还没到账

因为这类支付方式可能是异步确认。  
当前实现只在以下条件满足时履约：

- `checkout.session.completed` 且 `payment_status == paid`
- 或收到 `checkout.session.async_payment_succeeded`

### 9.4 Webhook 返回签名错误

检查：

- API 进程里的 `STRIPE_WEBHOOK_SECRET` 是否和当前测试入口匹配
- 你切换了 Stripe CLI 或新建了 Dashboard endpoint 后，是否忘了更新 secret
- 更新 secret 后是否重启了 API

### 9.5 cloudflared 地址变了以后收不到回调

Quick Tunnel 地址是临时的。  
地址变了，就要同时更新：

- Stripe Dashboard / Workbench 里的 endpoint URL
- 必要时重新确认 endpoint secret

---

## 10. 建议的测试记录模板

```text
测试方式：
- Stripe CLI / cloudflared

测试环境：
- API:
- Web:
- Stripe mode:

配置确认：
- STRIPE_SECRET_KEY:
- STRIPE_SUCCESS_URL:
- STRIPE_CANCEL_URL:
- STRIPE_WEBHOOK_SECRET:
- stripe_allowed_payment_methods:

执行结果：
- 成功支付:
- 取消支付:
- 失败支付:
- 异步支付:

验证结果：
- Webhook 到达:
- payments.status:
- users.expiresAt:
- Emby 恢复:
- Bot 通知:

发现问题：
- 
```

---

## 11. 相关文档

- [测试指南](./testing.md)
- [手工测试清单](./manual-testing-checklist.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)
- [配置参考](../reference/configuration-reference.md)
- [系统架构](../system-architecture.md)

