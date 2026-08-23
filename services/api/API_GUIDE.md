# Ember API 补充说明

这份文档不是主入口，也不再维护一份“完整 API 文档”。它只保留本地开发时仍有价值的补充信息。

主入口请先看：

- [服务入口 README](./README.md)
- [系统架构](/Users/konghang/data/me/github/ember/docs/system-architecture.md)
- [API 响应规范](/Users/konghang/data/me/github/ember/docs/reference/api-response-standard.md)

## 本地启动

```bash
cd services/api
cp .env.example .env
go run ./cmd/ember api
```

健康检查：

```bash
curl http://localhost:8080/health
```

## 调试时最常用的命令

### 编译与测试

```bash
go vet ./...
go test ./...
go build ./...
```

### 直接跑二进制

```bash
go build -o bin/ember ./cmd/ember
./bin/ember api
```

### 用脚本做最小接口冒烟

```bash
./test_api.sh
```

## 需要优先确认的环境变量

- `DATABASE_URL`
- `JWT_SECRET`
- `CONFIG_ENCRYPTION_KEY`
- `EMBY_URL`
- `EMBY_API_KEY`
- `TMDB_API_KEY`
- `INTERNAL_API_SECRET`

完整边界见 [配置参考](/Users/konghang/data/me/github/ember/docs/reference/configuration-reference.md)。

## API 调试建议

### 获取管理员 Token

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.token')
```

### 带 Token 调用受保护接口

```bash
curl http://localhost:8080/api/v1/admin/users \
  -H "Authorization: Bearer $TOKEN"
```

### 关注日志

开发时优先看：

- 启动期配置解析失败
- 数据库连接错误
- Emby / Stripe / Telegram / MoviePilot 调用错误

## 不再放在这里的内容

- 旧版“已实现 API 清单”
- 迁移阶段进度统计
- 过时的 `/api/v1/admin/login` 等旧路径说明
- 跟主文档重复的完整接口描述

## 相关文档

- [系统架构](/Users/konghang/data/me/github/ember/docs/system-architecture.md)
- [部署指南](/Users/konghang/data/me/github/ember/docs/runbooks/deployment.md)
- [测试指南](/Users/konghang/data/me/github/ember/docs/runbooks/testing.md)
