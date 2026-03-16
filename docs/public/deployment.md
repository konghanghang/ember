# 部署说明

这份文档面向部署者，只描述公开可用的标准部署路径。

## 当前推荐方式

使用仓库中的 Docker Compose 部署：

```bash
cd infrastructure/docker
cp .env.example .env
docker compose pull
docker compose up -d
```

## 部署前准备

- PostgreSQL
- Emby Server
- Telegram Bot 所需 token 和 webhook 地址
- 对外可访问的域名或反向代理

## 部署后最小检查

```bash
curl http://localhost:8080/health
curl http://localhost:8000/health
```

并确认：

- Web 页面可打开
- API 可返回健康检查
- Bot 服务可返回健康检查

## 继续阅读

- [配置说明](./configuration.md)
- [常见问题](./faq.md)
- 如果你需要内部部署细节，请看内部文档中心中的 runbooks
