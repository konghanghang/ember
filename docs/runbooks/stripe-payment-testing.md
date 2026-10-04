# Stripe 支付测试指南

这份手册只回答一件事：在 Ember 里怎么把 Stripe 支付链路测通。

目标链路：

1. 用户在续费中心发起购买
2. API 创建 Stripe Checkout Session
3. 用户在 Stripe 测试页完成或取消支付
4. Stripe Webhook 回调 Ember API
5. Ember 更新订单状态，按订单快照发放对应分组权益，并更新生效分组与期限

不管你用 `Stripe CLI`、`cloudflared` 还是 `ngrok`，真正算测通的标准都一样：**Webhook 到达并完成履约**。前端跳回 `success=true` 只能说明跳转成功，不能代替后端履约。

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
| `STRIPE_SECRET_KEY` | 设置中心 | Stripe 沙盒服务端密钥，使用 `sk_test_...` |
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
- 完成分组权益等级与媒体库包含校验；套餐目录不按用户当前组过滤
- 选择对测试用户有增益的套餐；同组续期，跨组独立计时，永久覆盖的无增益购买会被拒绝

### 3.3 Stripe Dashboard 准备

- 切到 Stripe `test mode`
- 如果要测动态支付方式，先在 Stripe Dashboard 开启对应支付方式
- 如果只是先测主链路，建议把系统里的 `stripe_allowed_payment_methods` 限制成：

```json
["card"]
```

这样最少变量，先把信用卡主路径跑通。

### 3.4 选择回调接入方式

| 方式 | 使用条件 | Webhook 签名密钥来源 |
|------|----------|---------------------|
| Stripe CLI | CLI 已登录，使用 `stripe listen` 转发 | 当前 listener 输出 |
| cloudflared | Quick Tunnel 成功连接边缘节点 | Stripe 沙盒 Dashboard 对应端点 |
| ngrok | 账号已配置，HTTP 隧道在线，可用本地 Inspect/Replay | Stripe 沙盒 Dashboard 对应端点 |

三种方式选择一种即可。公网隧道负责到 API 的回调，浏览器返回前端的地址仍独立配置；不要把 CLI 密钥与 Dashboard 端点密钥混用。

---

## 4. 成功标准

一次成功的测试至少要满足这几条：

- 用户能从 Ember 跳转到 Stripe Checkout
- Stripe 支付完成后，Webhook 成功打到 `POST /api/v1/webhooks/stripe`
- 对应 `payments.status` 变成 `completed`
- 对应分组权益按订单快照发放；同组限时续期增加期限，跨组保留原权益，永久不能按“延长 0 天”判断
- 当前生效组和 Emby 权限符合最高等级有效权益；因到期停用的访问恢复，人工限制不自动解除
- 已付款但无法自动发放的订单进入 `paid_review`，保留付款事实并等待人工处理

如果只看到前端跳回 `?success=true`，但 `payments` 还是 `pending`，这次测试就是失败。

---

## 5. 方式一：使用 Stripe CLI

适合使用 Stripe CLI 管理登录与本地事件转发；它的签名密钥与 Dashboard Webhook 端点密钥不同。

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

### 5.4 发起真实沙盒 Checkout 支付

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
- 对应分组权益是否正确发放，其他分组权益是否保持

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

仅生成公网地址不代表连接成功，还应看到 `Registered tunnel connection`。HTTP/2 使用 TCP 7844，QUIC 使用 UDP 7844；连接报 TLS EOF 或超时时，参照 [Cloudflared 联调排障](./cloudflared-local-testing.md)，不要直接归因于 Fake-IP。

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

- 新建 endpoint 时，使用 Stripe 为该端点生成的 `whsec_...`，写入 API 环境变量 `STRIPE_WEBHOOK_SECRET` 后重启 API。
- 只修改同一 endpoint 的 URL 且未轮换签名密钥时，可以沿用已有密钥，无需因域名变化重启 API。
- 从 Stripe CLI 切换到 Dashboard endpoint 时，不能沿用 CLI listener 的签名密钥。

### 6.5 发起支付与验证

后续步骤和 Stripe CLI 方案完全一样：

1. 进入 `/console/renewal`
2. 选择方案
3. 跳转 Stripe Checkout
4. 用测试卡支付
5. 检查前端回跳、Webhook 日志、支付记录和到期时间

---

## 7. 方式三：使用 ngrok

适合需要真实公网 Webhook、希望通过本地检查页面观察和重放请求的场景。本项目于 2026-10-04 使用 ngrok 3.39.11 完成 Stripe 沙盒端到端验证；不要求同时运行 Stripe CLI 或 cloudflared。

### 7.1 安装与账号配置（首次使用）

macOS：

```bash
brew install ngrok
ngrok version
```

登录 [ngrok 控制台](https://dashboard.ngrok.com/get-started/your-authtoken)，按控制台指引将自己的 Authtoken 保存到本机配置：

```bash
ngrok config add-authtoken 'YOUR_NGROK_AUTHTOKEN'
```

已经配置过时直接进入启动步骤。Authtoken 属于 ngrok 客户端凭据，与 Stripe 的服务端密钥、Webhook 签名密钥无关；不要写入项目文件或验收报告。

### 7.2 转发到 API

确认本地 API 已运行在 `8080`，在独立终端执行：

```bash
ngrok http 8080
```

保持终端运行，使用输出中 `Forwarding` 对应的 HTTPS 地址。不要把隧道指向前端 `3000` 或 Bot `8000`。公网域名后缀可能不同，以下只是占位示例，以本机实际输出为准：

```text
https://your-endpoint.ngrok-free.dev
```

### 7.3 更新 Stripe 沙盒 Webhook

在与 `STRIPE_SECRET_KEY` 对应的沙盒中，编辑已有端点或新建端点，填写完整地址：

```text
https://your-endpoint.ngrok-free.dev/api/v1/webhooks/stripe
```

订阅第 6.3 节列出的三个 Checkout 事件。只修改已有端点的 URL 且密钥未轮换时，本地 `STRIPE_WEBHOOK_SECRET` 不变；新建端点或更换沙盒时，应重新核对该端点的签名密钥。

需要更新密钥时，写入本地 `services/api/.env` 或 GoLand 的进程环境，再重启 API。进程环境变量优先于 `.env`；如果两处都配置了，需避免 GoLand 中的旧值覆盖文件。

支付成功和取消地址仍指向本地前端，例如 `http://localhost:3000/console/renewal?success=true`，不改成隧道地址。隧道域名变化后更新 Stripe 端点 URL，不必重新建立整个端点。

### 7.4 观察回调与重放

默认本地检查页面为 `http://127.0.0.1:4040`；如端口被调整，以 ngrok 输出的 `Web Interface` 为准。

1. 从 Ember 续费中心下单，确认 Stripe 收银台标注沙盒，再使用第 5.5 节测试卡支付。
2. 在检查页面找到 `POST /api/v1/webhooks/stripe`，核对事件类型、沙盒标志 `livemode=false` 和响应状态；结合 API 日志及 Ember 订单/权益判断履约结果。
3. 选择刚刚成功的测试回调，点击 **Replay**，核对后端幂等返回、订单不回退、权益不重复增加。

Replay 使用原始请求和签名，应在签名有效时间窗口内执行；隔很久再重放可能因时间戳过期而验签失败，不能据此判定幂等失效。需要验证晚些时候的重复投递时，使用 Stripe 沙盒端点的重新发送功能生成新签名。

检查页面含完整请求和签名，只在本机查看；项目报告仅保留脱敏事件类型、状态、数量和权益变化，不保存完整请求、密钥或签名。验收结束后用 `Ctrl+C` 停止自己启动的隧道；自动化验收不能擅自停止用户保留运行的隧道。

---

## 8. 推荐测试顺序

不要一上来就把所有支付方式一起测。按这个顺序做，最省时间：

### 8.1 第一轮：信用卡主链路

- `stripe_allowed_payment_methods = ["card"]`
- 跑一次成功支付
- 跑一次取消支付
- 跑一次拒付卡

### 8.2 第二轮：异步支付方式

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

## 9. 每次测试都要看的检查项

### 9.1 前端

- `/console/renewal` 方案列表是否正常展示
- 点击购买后是否跳转 Stripe
- `?success=true` / `?canceled=true` 提示是否符合预期

### 9.2 API

- `POST /api/v1/payments/checkout` 是否返回 URL
- `POST /api/v1/webhooks/stripe` 是否收到请求
- Webhook 验签是否通过

### 9.3 数据

- `payments.stripeSessionId`
- `payments.stripePaymentIntentId`
- `payments.status`
- 持有权益、生效分组和 `users.expiresAt`

### 9.4 外部副作用

- 因权益到期停用的 Emby 访问是否恢复，人工限制是否保留
- 如果配置了 Bot 通知，管理员是否收到支付成功通知

---

## 10. 常见问题

### 10.1 看到 `success=true`，但没有续期

原因通常只有三个：

- `STRIPE_WEBHOOK_SECRET` 配错了
- Webhook 根本没有打到本地 API
- Webhook 打到了，但事件对应不到本地 `payments.stripeSessionId`

先查 API 日志，不要先怀疑前端。

### 10.2 重复点购买，为什么还是老的 Stripe 页面

这是正常行为。Ember 会复用同一用户、同一方案、30 分钟内未过期的待支付订单。

处理方式：

- 换一个方案测试
- 等旧订单过期
- 或者先清理测试数据再测

### 10.3 测支付宝或微信支付时，为什么成功页已经回来了，但权益还没到账

因为这类支付方式可能是异步确认。  
当前实现只在以下条件满足时履约：

- `checkout.session.completed` 且 `payment_status == paid`
- 或收到 `checkout.session.async_payment_succeeded`

### 10.4 Webhook 返回签名错误

检查：

- API 进程里的 `STRIPE_WEBHOOK_SECRET` 是否和当前测试入口匹配
- 你切换了 Stripe CLI 或新建了 Dashboard endpoint 后，是否忘了更新 secret
- 更新 secret 后是否重启了 API

### 10.5 cloudflared 地址变了以后收不到回调

Quick Tunnel 地址是临时的。  
地址变了，就要同时更新：

- Stripe Dashboard / Workbench 里的 endpoint URL
- 必要时重新确认 endpoint secret

---

## 11. 建议的测试记录模板

```text
测试方式：
- Stripe CLI / cloudflared / ngrok

测试环境：
- API:
- Web:
- Stripe mode:

配置确认（只记是否已设置、来源及是否匹配，不记录密钥值）：
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

## 12. 相关文档

- [测试指南](./testing.md)
- [手工测试清单](./manual-testing-checklist.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)
- [配置参考](../reference/configuration-reference.md)
- [系统架构](../system-architecture.md)

## 13. 2026-10-04 Stripe 沙盒页面验收

通过真实 Stripe 沙盒 Checkout 与 ngrok 回调完成验收，未使用真实银行卡或真实扣款；全程从 Ember 页面下单与管理权益，未直接改数据库。收银台显示沙盒，回调事件 livemode=false；后端收到 checkout.session.completed，成功履约返回 200。

- 同组连续两笔 30 天付款：增强期限由 11 月 3 日累加到 12 月 3 日，基础永久权益保持。
- 取消 180 天订单：页面返回取消提示，权益不变，订单保持待支付并可继续付款。
- 官方拒付测试卡：Stripe 明确拒绝，Ember 权益不变；同一组合订单改用成功测试卡后完成支付，基础限时转永久，增强独立发放 30 天。
- ngrok 检查页面重放成功事件：Webhook 200，日志记录已处理事件幂等返回，期限不重复增加。
- 订单快照：按 30 天下单后，把测试商品临时改为 60 天；付款仍仅增加 30 天。商品随后恢复 30 天。
- 付款前人工发放了相同永久权益：付款事实保留，订单进入 paid_review / already_owned；后台补偿增强组 1 天后状态 resolved。再次重放原成功事件，状态保持已人工处理。
- 最终保留 6 笔测试订单：4 笔完成、1 笔已人工处理、1 笔取消返回后待支付；5 笔沙盒付款成功，每笔 USD 9.99。保留两位测试用户及权益供复查。

限制：本地 Bot 的 localhost:8000 未运行，支付通知连接失败，未验证 Telegram 实际送达；不影响已证实的付款履约和 Policy 同步。未验证支付宝及异步支付方式、3DS、真实退款；不能把银行卡同步支付结果扩展为这些路径通过。

## 单组商品收敛复验（2026-10-04）

原 SQL 与测试库转换后，通过页面新建单组永久 E 商品，再编辑为增强分组 60 天并完成 Stripe 沙盒付款。订单完成且 `payments.benefits` 恰好一项；测试用户增强期限增加 60 天，基础永久权益保持。E 商品验收后已下架，测试订单和权益保留。

D 商品按用户确认改为增强 30 天并下架，其历史 completed 订单仍保留“增强 30 天＋基础永久”两项快照；后台历史表仍显示两项。此次新增成功付款验证使用既有 ngrok Webhook 入口，没有真实扣款，也没有重新运行首轮全部拒付/人工处理场景；相关幂等、迟到回调和人工流程由自动化回归保护。
