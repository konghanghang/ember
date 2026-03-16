# Telegram Bot 命令菜单方案（已落地）

## 背景

Telegram 命令菜单不是只有一份“全局配置”，而是带作用域优先级：

`BotCommandScopeChat` > `BotCommandScopeChatAdministrators` / `BotCommandScopeAllPrivateChats` / `BotCommandScopeAllGroupChats` > `BotCommandScopeDefault`

之前的问题有两层：

1. Bot 虽然注册了 `CommandHandler`，但早期没有正确同步 Telegram 命令菜单
2. 后续即使补了 `set_my_commands`，历史上残留在更高优先级 scope 的旧命令，仍会覆盖新菜单

最终目标很明确：

- 私聊显示正确的命令菜单
- 群聊默认不显示命令菜单
- 历史残留旧群菜单时，按群清理一次
- 不做高频重复同步，不引入过重的数据库状态设计

---

## 当前方案

### 1. 启动时同步私聊菜单

**文件**：`services/bot/app/server.py`

启动流程里会执行 `sync_bot_commands()`，当前行为是：

1. 先删除 `BotCommandScopeDefault`
2. 只向 `BotCommandScopeAllPrivateChats` 写入命令菜单
3. best-effort 清理这些旧作用域：
   - `BotCommandScopeAllGroupChats`
   - `BotCommandScopeChat(TELEGRAM_ADMIN_CHAT_ID)`（如果配置了）
   - `BotCommandScopeChat(TELEGRAM_GROUP_CHAT_ID)`（如果配置了）
   - `BotCommandScopeChatAdministrators(TELEGRAM_GROUP_CHAT_ID)`（如果配置了）

这样做的关键点是：

- **default scope 必须为空**
- 否则群聊清掉具体作用域后，会回退继承 default 命令，导致“群聊默认无菜单”失效

当前写入的私聊命令为：

- `/search`
- `/bind`
- `/info`
- `/redeem`
- `/resetpw`
- `/cancel`

### 2. 群聊按群懒同步

**文件**：`services/bot/app/menu_sync.py`

Bot 收到 Telegram Webhook 更新时，会先调用 `schedule_group_menu_sync()`。

它不会每条群消息都打 Telegram API，而是：

1. 只处理 `group` / `supergroup`
2. 通过进程内缓存 `_synced_chat_versions` 判断该群是否已经同步过当前 `BOT_MENU_VERSION`
3. 通过 `_in_flight_chats` 避免同一群并发重复清理
4. 对未同步过的群，只异步执行一次：
   - `delete_my_commands(BotCommandScopeChat(chat_id))`
   - `delete_my_commands(BotCommandScopeChatAdministrators(chat_id))`

同步成功后，会把该 `chat_id` 记到当前进程内存里。

### 3. 群管理员手动兜底

**文件**：`services/bot/app/handlers/telegram_handler.py`

新增群命令：

- `/refresh_menu`

规则：

1. 只能在群聊里使用
2. 配置中的管理员账号始终可用
3. 普通群管理员仅在 Bot 自己也是该群管理员时可校验并使用
4. 执行后会强制清理当前群的两个旧作用域：
   - `BotCommandScopeChat`
   - `BotCommandScopeChatAdministrators`
5. 同时会重试清理：
   - `BotCommandScopeDefault`
   - `BotCommandScopeAllGroupChats`

这个命令用于：

- 某个群还没等到“首次消息触发”
- 或想立刻验证群菜单是否已清掉
- 或启动期全局 scope 清理失败后，手动补救群继承到的旧菜单

### 4. 命令帮助文案策略

**文件**：`services/bot/app/handlers/telegram_handler.py`

除了命令菜单本身，当前还同步约束了命令的错误提示方式，避免用户只发 `/bind`、`/redeem` 这类不带参数的命令后，不知道下一步该做什么。

当前帮助文案采用统一结构：

1. 先说明错误原因
2. 再给“正确用法”
3. 再给一条可直接照抄的“示例”
4. 如有前置步骤，再补“下一步怎么做”

当前已覆盖这些命令：

- `/bind`
- `/redeem`
- `/resetpw`
- `/search`

示例风格：

```text
❌ 缺少绑定验证码或格式不正确

正确用法：
/bind 123456

示例：
/bind 123456

操作步骤：
1. 打开 Ember 控制台
2. 生成 Telegram 绑定验证码
3. 把 6 位数字填在 /bind 后面发送

验证码必须是 6 位数字。
```

这样做的目的不是“把提示写长”，而是把提示改成用户可执行的下一步：

- `/bind`：明确告诉用户先去控制台生成验证码
- `/redeem`：明确告诉用户兑换码应该直接写在命令后面
- `/resetpw`：明确告诉用户后面的内容就是新密码本身
- `/search`：明确告诉用户必须填写关键词，并给出搜索示例

额外约束：

- 不再使用容易误导的占位写法，如 `/bind <code>验证码</code>`
- 优先使用用户可复制的真实示例，如 `/bind 123456`
- “格式错误”和“前置条件未满足”分开提示，避免用户看完还是不知道去哪操作

---

## 为什么没有做数据库持久化

这次最终采用的是**轻量方案**，不额外引入表和内部 API。

原因很现实：

- 当前 Bot 主要是自用，不是高并发大规模运营
- 群数量少
- 可以接受 Bot 重启后，同一群再次触发一次菜单清理

当前权衡是：

- 用进程内缓存换简单性
- 保留 `/refresh_menu` 作为人工兜底
- 不为了避免“重启后再清一次”引入整套数据库状态管理

---

## 当前涉及文件

| 文件 | 作用 | 状态 |
|------|------|------|
| `services/bot/app/server.py` | 启动时同步私聊菜单，接入群聊懒同步入口 | ✅ 已完成 |
| `services/bot/app/menu_sync.py` | 群聊首次发现按群清理旧菜单作用域，进程内去重 | ✅ 已完成 |
| `services/bot/app/handlers/telegram_handler.py` | 提供 `/refresh_menu` 手动刷新命令 | ✅ 已完成 |
| `services/bot/app/formatters/message_formatter.py` | 绑定成功消息已精简，不再重复列出命令 | ✅ 已完成 |

---

## 当前验证方式

1. 语法检查：

```bash
PYTHONPYCACHEPREFIX=/tmp/ember-bot-pyc python3 -m py_compile \
  services/bot/app/server.py \
  services/bot/app/menu_sync.py \
  services/bot/app/handlers/telegram_handler.py \
  services/bot/app/clients/api_client.py
```

2. 启动后检查日志：
   - `Bot 命令菜单已同步`
   - `群菜单作用域已同步: chat_id=...`

3. 私聊验证：
   - 点击 `/` 按钮，应看到正确私聊命令菜单

4. 群聊验证：
   - 默认应不显示命令菜单
   - 如仍有旧菜单，在群里发送一条消息或执行 `/refresh_menu`

---

## 已知边界

1. 菜单同步状态只保存在 Bot 进程内存中
2. Bot 重启后，同一群再次收到第一条消息时，可能会重新清理一次
3. 这是当前轻量方案的预期行为，不算故障
4. 如果未来 Bot 变成多实例、群很多、或需要审计同步状态，再考虑升级为持久化方案
