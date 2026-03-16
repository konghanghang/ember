# Emby 集成

Ember 的核心能力围绕 Emby 展开。

## Ember 会使用 Emby 做什么

- 用户创建与密码同步
- 账号禁用/解禁
- 媒体统计
- 最近入库
- 活跃会话
- 设备管理
- 追剧日历相关数据

## 关键配置

- `EMBY_URL`
- `NEXT_PUBLIC_EMBY_URL`
- `EMBY_API_KEY`
- `EMBY_WEBHOOK_TOKEN`（按需）

## 注意事项

- `EMBY_URL` 和 `NEXT_PUBLIC_EMBY_URL` 不一定相同
- 对外展示地址和内部服务访问地址不要混用
- Emby 不可用时，很多媒体相关能力都会受影响

## 继续阅读

- [配置说明](../configuration.md)
- [追剧日历](../features/tv-calendar.md)
