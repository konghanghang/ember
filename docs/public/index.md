---
layout: home

hero:
  name: Ember
  text: Emby 用户管理系统公开文档
  tagline: 面向用户和部署者的使用、部署与集成说明
  actions:
    - theme: brand
      text: 快速开始
      link: /getting-started
    - theme: alt
      text: 功能总览
      link: /features/overview

features:
  - title: 快速部署
    details: 基于 Docker Compose 的标准部署路径，适合快速拉起 API、Web、Bot 和 PostgreSQL。
  - title: 用户与运营
    details: 覆盖注册、续期、兑换码、支付、求片订阅、播放排行和追剧日历。
  - title: 外部集成
    details: 提供 Emby、Telegram、Stripe、MoviePilot 的公开配置与接入说明。
---

# Ember 文档

Ember 是一个面向 Emby 的用户管理系统，提供注册登录、账号生命周期管理、兑换码、支付、求片订阅、Telegram Bot 和追剧日历等能力。

这套文档只面向公开发布：

- 介绍 Ember 是什么
- 说明如何部署和配置
- 解释用户和管理员能做什么
- 描述与 Emby、Telegram、Stripe、MoviePilot 的集成方式

不会放进这里的内容：

- 内部架构细节
- 实施方案与重构提案
- 历史归档
- AI 协作规则

## 从这里开始

- [快速开始](./getting-started.md)
- [部署说明](./deployment.md)
- [配置说明](./configuration.md)
- [功能总览](./features/overview.md)
- [Telegram 集成](./integrations/telegram.md)
- [管理后台](./admin/overview.md)
- [常见问题](./faq.md)

## 适用对象

- 想了解 Ember 能做什么的用户
- 准备部署 Ember 的管理员
- 需要接入 Emby、Telegram、Stripe 或 MoviePilot 的维护者

## 说明

这套目录只用于公开发布，不包含内部架构、方案设计、归档和 AI 协作规则。
