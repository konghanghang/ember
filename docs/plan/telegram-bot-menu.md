# Telegram Bot 命令菜单设置

## 背景

当前 Bot 在 `server.py` 中通过 `CommandHandler` 注册了 4 个用户命令（bind / info / redeem / resetpw），但从未调用 `bot.set_my_commands()`，导致用户在 Telegram 客户端看不到命令菜单（输入框左下角的 "/" 按钮无命令列表）。

同时，绑定成功消息（`format_bind_success`）中硬编码了命令列表作为引导，有了菜单后这部分冗余，应精简。

## 方案

两处改动，两个文件。

### 改动 1：注册命令菜单

**文件**：`services/bot/app/server.py`

在 `lifespan` 函数中，`tg_app.start()` 之后、webhook 注册之前，添加：

```python
from telegram import BotCommand

# lifespan 函数内，await tg_app.start() 之后添加：
await tg_app.bot.set_my_commands([
    BotCommand("bind", "绑定 Ember 账户"),
    BotCommand("info", "查看账户信息"),
    BotCommand("redeem", "兑换码"),
    BotCommand("resetpw", "重置密码"),
])
logger.info("Bot 命令菜单已注册")
```

### 改动 2：精简绑定成功消息

**文件**：`services/bot/app/formatters/message_formatter.py`

`format_bind_success` 函数（第 82-90 行）去掉命令列表，只保留绑定成功的核心信息：

```python
def format_bind_success(data: dict) -> str:
    username = escape(str(data.get("username", "") or ""))
    return (
        "✅ <b>绑定成功</b>\n\n"
        f"👤 已绑定账号：<b>{username}</b>"
    )
```

## 涉及文件

| 文件 | 改动 |
|------|------|
| `services/bot/app/server.py` | 新增 `set_my_commands` 调用 + import |
| `services/bot/app/formatters/message_formatter.py` | 精简 `format_bind_success` 返回内容 |

## 验证方式

1. `cd services/bot && python -c "from app.server import app; print('import ok')"` 验证语法
2. 部署后在 Telegram 客户端打开 Bot 对话，点击输入框左下角 "/" 按钮，应能看到 4 个命令及其描述
3. 执行 `/bind` 绑定成功后，确认消息只显示绑定结果，不再列出命令
