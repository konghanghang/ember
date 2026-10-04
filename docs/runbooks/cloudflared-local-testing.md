# Cloudflared 本地联调指南

> 本文保留 Telegram webhook 的 Cloudflared 配置与网络排障。Stripe 的 API 8080 回调以及 Stripe CLI / Cloudflared / ngrok 操作统一见 [Stripe 支付测试指南](./stripe-payment-testing.md)。

---

## 1. 适用场景

- 本地运行 `services/bot`（默认 `8000` 端口）
- Telegram 需要公网 HTTPS webhook 回调地址
- 本机使用 Surge，可能拦截 QUIC 或使用 Fake-IP

---

## 2. 前置条件

### 2.1 安装 cloudflared（macOS）

```bash
brew install cloudflared
cloudflared --version
```

### 2.2 必填环境变量

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_WEBHOOK_SECRET`
- `INTERNAL_API_SECRET`
- `WEBHOOK_URL`（由 cloudflared 提供）
- `API_URL`（本地通常 `http://localhost:8080`）
- `BOT_NOTIFY_URL`（本地通常 `http://localhost:8000`）

可用以下命令生成密钥：

```bash
openssl rand -hex 32
```

---

## 3. 快速开始（推荐命令）

先启动 Bot，再开隧道。为避免 QUIC 被代理拦截，强制使用 HTTP/2：

```bash
cloudflared tunnel --protocol http2 --edge-ip-version 4 --url http://localhost:8000
```

命令输出里会出现一个地址，例如：

```text
https://xxxx.trycloudflare.com
```

把这个地址设置为：

```bash
WEBHOOK_URL=https://xxxx.trycloudflare.com
```

先确认日志出现 `Registered tunnel connection`；仅生成域名不代表隧道已连通。然后重启 Bot（Bot 启动时会注册 webhook）。

---

## 4. 已确认使用 Surge 时的排查参考

以下步骤只针对已确认由本机 Surge 接管的环境，不能直接套用到普通 DNS 或旁路由 PassWall。TLS EOF 本身不能证明 Fake-IP 有问题；是否直连应结合当前出口的实测连通性决定。

### 4.1 规则直连（放在前面）

```ini
[Rule]
PROCESS-NAME,cloudflared,DIRECT
DOMAIN-SUFFIX,argotunnel.com,DIRECT
DOMAIN-SUFFIX,trycloudflare.com,DIRECT
DOMAIN-SUFFIX,cloudflare.com,DIRECT
```

### 4.2 DNS 排除 Fake-IP

```ini
[DNS]
fake-ip-filter = *.argotunnel.com,*.trycloudflare.com,argotunnel.com,trycloudflare.com
```

### 4.3 如开启 MITM，排除相关域名

```ini
[MITM]
hostname = -argotunnel.com,-*.argotunnel.com,-trycloudflare.com,-*.trycloudflare.com
```

### 4.4 清理代理环境变量（可选但建议）

```bash
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy
```

---

## 5. 验证步骤

### 5.1 Bot 健康检查

```bash
curl http://localhost:8000/health
```

### 5.2 查看 Telegram webhook 注册状态

```bash
curl "https://api.telegram.org/bot<TELEGRAM_BOT_TOKEN>/getWebhookInfo"
```

确认 `url` 是当前 `WEBHOOK_URL/telegram/webhook`。

### 5.3 端到端检查

1. 用户在 Web 提交订阅
2. 管理员 Telegram 收到消息与按钮
3. 点击按钮后消息更新为“已通过/已拒绝”
4. Web 控制台订阅状态同步变化

---

## 6. 常见问题

### 6.1 `TLS handshake with edge error: EOF`

这表示到 Cloudflare 边缘节点的 TLS 握手被中断，尚未到达本地 Bot/API。先区分节点地址是否为虚拟地址，再检查实际路由、代理规则命中及节点出口。只有确认使用 Surge 时才参考第 4 节；真实 IP 环境也可能发生 EOF，不能直接判定为 Fake-IP 或证书问题。

### 6.2 QUIC 被拦截

处理：使用 `--protocol http2` 启动，不依赖 QUIC。

### 6.3 webhook 返回 401

原因：`TELEGRAM_WEBHOOK_SECRET` 与 Bot 注册 webhook 使用的密钥不一致。  
处理：统一密钥后重启 Bot 重新注册 webhook。

### 6.4 隧道地址变化后收不到回调

原因：Quick Tunnel 地址是临时的。  
处理：更新 `WEBHOOK_URL` 并重启 Bot。

### 6.5 非 Fake-IP、旁路由 PassWall 环境

- 本机未开启系统代理不代表直连：默认网关可能是运行 PassWall 的旁路由。
- HTTP/2 需要到边缘节点的 TCP 7844，QUIC 需要 UDP 7844。转发所有端口只证明端口范围覆盖，不能证明连接命中了代理或所选节点可达。
- 添加域名代理规则后仍失败时，应核对实际目标 IP 的路由日志；cloudflared 发现边缘节点后按 IP 连接，不能仅凭域名已加入列表声称分流生效。
- `Failed to fetch features, default to disable` 或 `Unable to lookup protocol percentage` 是探测失败信息；仍需看后续边缘连接日志，不能单独当成隧道最终失败原因。
- 2026-10-04 本地排查中，真实边缘 IP 的 TCP 可连接，但 TLS 1.2/1.3 均 EOF，QUIC 多节点超时；功能域名的普通 DNS TXT 查询超时而 HTTPS DNS 查询成功。未取得旁路由规则命中及出口对照证据，具体故障点未证实，不记录为 PassWall 或 Fake-IP 的确定缺陷。
- 同一轮 Stripe 验收改用 ngrok 后，真实沙盒回调与重放通过；这不代表 Cloudflared 问题已解决。操作入口见 [Stripe 支付测试指南](./stripe-payment-testing.md)。
