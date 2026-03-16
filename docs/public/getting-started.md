# 第一次使用

这份文档不是部署指南，只回答一个问题：

第一次登录 Ember 之后，你应该先看什么、先做什么。

## 适合什么场景

如果你想要的是下面这类能力，Ember 就是对路的：

- 管理 Emby 用户账号、有效期和启停状态
- 支持开放注册或兑换码注册
- 支持兑换码续期和 Stripe 支付
- 通过 Telegram Bot 提供绑定、查询、兑换和通知
- 提供求片订阅、播放排行、追剧日历等运营能力

如果你只需要一个纯静态的 Emby 登录页，或者完全不打算接 Telegram / 支付 / 运营功能，那 Ember 可能偏重。

## 第一次登录先做这三件事

### 1. 先看我的账号

打开 [我的账号](./features/my-account.md)，确认：

- 用户名是否正确
- Emby ID 是否已经关联
- 当前账号是否有效
- 还剩多少天

如果你一上来就发现自己已经过期，不要乱翻，直接去 [续费中心](./features/payments.md)。

### 2. 决定你接下来要做什么

你通常会落到下面几条路径之一：

- 想先知道装哪个播放器：先看 [播放器推荐](./features/players.md)
- 想继续看片：先看 [媒体库](./features/library.md)
- 想找热门内容：先看 [播放排行榜](./features/rankings.md)
- 想申请想看的片：先看 [订阅管理](./features/subscriptions.md)
- 想追连载：先看 [追剧日历](./features/tv-calendar.md)
- 想恢复或延长账号：先看 [续费中心](./features/payments.md)

### 3. 如果你要用 Bot，尽早绑定

打开 [Telegram Bot](./features/telegram-bot.md)，按文档完成绑定。绑定后，很多查询和轻量操作都可以直接在聊天窗口里完成。

## 最短熟悉路径

如果你只想用最短时间把整站摸清，建议按这个顺序读：

1. [控制台总览](./features/user-console.md)
2. [我的账号](./features/my-account.md)
3. [续费中心](./features/payments.md)
4. [Telegram Bot](./features/telegram-bot.md)

## 常见第一步误区

- 账号过期时，不要以为系统坏了，先去看续费中心
- 媒体库不是完整搜索页，它更适合看最近入库
- 订阅管理是“提需求”，不是“立即观看”
- Bot 是第二入口，不是主站替代品

## 建议阅读顺序

- [功能总览](./features/overview.md)
- [控制台总览](./features/user-console.md)
- [Telegram Bot](./features/telegram-bot.md)
