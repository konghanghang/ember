# Cloudflared 本地联调指南

> 适用于 Telegram webhook 本地联调（Bot 在本机，公网入口由 cloudflared 提供）

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

然后重启 Bot（Bot 启动时会注册 webhook）。

---

## 4. Surge 配置（关键）

如果遇到 `TLS handshake with edge error: EOF` 且日志出现 `ip=198.18.x.x`，通常是 Surge Fake-IP/代理链路影响。

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

现象：`cloudflared` 日志里出现 `ip=198.18.x.x`。  
原因：请求仍在走 Surge Fake-IP/代理链路。  
处理：按第 4 节配置直连 + Fake-IP 排除，并使用 `--protocol http2`。

### 6.2 QUIC 被拦截

处理：使用 `--protocol http2` 启动，不依赖 QUIC。

### 6.3 webhook 返回 401

原因：`TELEGRAM_WEBHOOK_SECRET` 与 Bot 注册 webhook 使用的密钥不一致。  
处理：统一密钥后重启 Bot 重新注册 webhook。

### 6.4 隧道地址变化后收不到回调

原因：Quick Tunnel 地址是临时的。  
处理：更新 `WEBHOOK_URL` 并重启 Bot。

