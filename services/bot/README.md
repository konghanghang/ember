# Ember Telegram Bot

> Emby 用户管理系统的 Telegram Bot 服务（Monorepo 架构）

**项目位置**: `services/bot/`

## 🎯 技术栈

- **语言**: Python 3.11+
- **框架**: python-telegram-bot
- **HTTP 客户端**: httpx (异步)
- **配置**: python-dotenv

## 📦 开发状态

🚧 **待实现** - Telegram Bot 正在开发中

计划功能：
- [ ] 用户注册（使用邀请码）
- [ ] 账号信息查询
- [ ] 到期时间提醒
- [ ] 订阅管理
- [ ] 管理员命令

## 🚀 快速开始

### 安装依赖

```bash
cd services/bot
pip install -r requirements.txt
```

### 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 填入 TELEGRAM_BOT_TOKEN
```

### 开发模式

```bash
python main.py
```

## 📚 Bot 命令设计

### 用户命令

- `/start` - 开始使用 Bot
- `/register <invite_code>` - 使用邀请码注册
- `/me` - 查看个人信息
- `/subscribe <tmdb_id>` - 订阅影视资源

### 管理员命令

- `/admin users` - 查看用户列表
- `/admin invite <count> <days>` - 生成邀请码
- `/admin extend <user_id> <days>` - 延长用户到期时间

## 📚 文档

- [迁移指南](../../docs/MIGRATION-GUIDE.md)
- [Bot 架构设计](../../docs/bot-architecture.md)（待创建）

---

Made with ❤️ by Kong Hang
