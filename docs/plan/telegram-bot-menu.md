# Telegram Bot 命令菜单设置

## 背景

当前 Bot 在 `server.py` 中通过 `CommandHandler` 注册了 4 个用户命令（bind / info / redeem / resetpw），但从未调用 `bot.set_my_commands()`，导致用户在 Telegram 客户端看不到命令菜单（输入框左下角的 "/" 按钮无命令列表）。

同时，绑定成功消息（`format_bind_success`）中硬编码了命令列表作为引导，有了菜单后这部分冗余，应精简。

## 方案

三处改动，两个文件。

### 改动 1：注册命令菜单（✅ 已完成）

**文件**：`services/bot/app/server.py`

在 `lifespan` 函数中，`tg_app.start()` 之后、webhook 注册之前，添加：

```python
from telegram import BotCommand

# lifespan 函数内，await tg_app.start() 之后添加：
await tg_app.bot.set_my_commands([
    BotCommand("search", "搜索影视"),
    BotCommand("bind", "绑定 Ember 账号"),
    BotCommand("info", "查看账号信息"),
    BotCommand("redeem", "兑换续期码"),
    BotCommand("resetpw", "重置密码"),
    BotCommand("cancel", "取消备注输入"),
])
logger.info("Bot 命令菜单已注册")
```

### 改动 2：精简绑定成功消息（✅ 已完成）

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

### 改动 3：覆盖高优先级 scope（✅ 已完成）

**问题**：改动 1 已部署且日志确认 `set_my_commands` 成功，但私聊中菜单仍显示旧命令（缺少 search、cancel）。

**原因**：Telegram `set_my_commands` 有 scope 优先级机制。旧命令可能通过 BotFather 或之前的 API 调用设置在了更高优先级的 scope（如 `BotCommandScopeAllPrivateChats`），当前代码只设置 `BotCommandScopeDefault`，无法覆盖高优先级 scope 的命令。

> Telegram scope 优先级：`BotCommandScopeChat`（特定聊天）> `BotCommandScopeAllPrivateChats`（所有私聊）> `BotCommandScopeAllGroupChats`（所有群组）> `BotCommandScopeDefault`（默认）

**文件**：`services/bot/app/server.py`

为避免“先删后设”在 Telegram API 临时失败时清空现有菜单，采用无破坏顺序：
1. 先设置 `BotCommandScopeDefault`
2. 再设置 `BotCommandScopeAllPrivateChats`（覆盖私聊高优先级 scope）
3. 最后仅对 `BotCommandScopeAllGroupChats` 做 best-effort 清理（失败忽略）

```python
from telegram import BotCommand, BotCommandScopeAllPrivateChats, BotCommandScopeAllGroupChats

commands = [
    BotCommand("search", "搜索影视"),
    BotCommand("bind", "绑定 Ember 账号"),
    BotCommand("info", "查看账号信息"),
    BotCommand("redeem", "兑换续期码"),
    BotCommand("resetpw", "重置密码"),
    BotCommand("cancel", "取消备注输入"),
]

await tg_app.bot.set_my_commands(commands)
await tg_app.bot.set_my_commands(commands, scope=BotCommandScopeAllPrivateChats())
with suppress(Exception):
    await tg_app.bot.delete_my_commands(scope=BotCommandScopeAllGroupChats())
logger.info("Bot 命令菜单已注册")
```

这样即使命令注册失败，也不会先删除旧菜单导致用户看到空命令列表。

## 涉及文件

| 文件 | 改动 | 状态 |
|------|------|------|
| `services/bot/app/server.py` | 新增 `set_my_commands` 调用 + import | ✅ 已完成 |
| `services/bot/app/formatters/message_formatter.py` | 精简 `format_bind_success` 返回内容 | ✅ 已完成 |
| `services/bot/app/server.py` | 设置 Default + AllPrivateChats 命令，并 best-effort 清理 AllGroupChats | ✅ 已完成 |

## 验证方式

1. `cd services/bot && python3 -m py_compile app/server.py` 验证语法
2. 部署后检查日志确认 `"Bot 命令菜单已注册"`
3. 在 Telegram 私聊中点击 `/` 按钮，应看到 6 个命令：search、bind、info、redeem、resetpw、cancel
4. 执行 `/bind` 绑定成功后，确认消息只显示绑定结果，不再列出命令
