# 发布说明

本目录存放每个正式版本的 Release Notes。`create-release.yml` 在收到 `v*` tag 后，会读取同名文件创建 GitHub Draft Release。

## 文件命名

- 稳定版：`v1.6.4.md`
- 预发布版：`v1.7.0-rc.1.md`

文件名必须与 Git tag 完全一致。比如 tag 是 `v1.6.4`，发布说明文件必须是：

```text
docs/releases/v1.6.4.md
```

## 发布前要求

打 tag 前必须先提交对应版本的发布说明文件。缺少文件时，`Create Release` workflow 会 fail-fast，不会创建草稿。

发布说明必须人工核对，尤其是：

- 用户可见功能和修复
- 数据库迁移和最低直升版本
- 配置项、环境变量、部署方式变化
- Docker 镜像 tag
- 需要人工操作或不兼容的升级事项

## 模板

新版本从 [`release-template.md`](./release-template.md) 复制，按本次实际改动保留必要章节。没有内容的章节可以删除。
