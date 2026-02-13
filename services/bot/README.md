# Ember Telegram Bot

Telegram 通知服务，负责两件事：

1. 接收 Go API 的新订阅通知并推送给管理员  
2. 接收 Telegram webhook 回调，点击按钮后调用 Go API 内部审批接口

## 目录结构

```
services/bot/
├── main.py
├── app/
│   ├── __init__.py
│   ├── config.py
│   ├── server.py
│   ├── clients/
│   │   ├── __init__.py
│   │   └── api_client.py
│   ├── handlers/
│   │   ├── __init__.py
│   │   └── telegram_handler.py
│   └── formatters/
│       ├── __init__.py
│       └── message_formatter.py
├── requirements.txt
└── Dockerfile
```

## 环境变量

必填：

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_WEBHOOK_SECRET`
- `INTERNAL_API_SECRET`
- `WEBHOOK_URL`

可选：

- `API_URL`（默认 `http://localhost:8080`）
- `BOT_PORT`（默认 `8000`）

## 本地运行

```bash
cd services/bot
pip install -r requirements.txt
python main.py
```

## HTTP 端点

- `GET /health`：健康检查
- `POST /notify/subscription`：Go API 通知入口（需 `X-Internal-Secret`）
- `POST /telegram/webhook`：Telegram webhook 入口
