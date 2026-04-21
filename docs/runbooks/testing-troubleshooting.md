# 测试排障

这份文档只处理测试时最常见的阻塞问题。

## 编译级验证失败

### API 失败

```bash
cd services/api
go vet ./...
go test ./...
go build ./...
```

优先检查：

- 依赖下载是否完整
- 新增字段或接口是否破坏现有编译面
- 配置读取是否引入未处理的空值

### Web 失败

```bash
cd services/web
npm ci
npm run build
```

优先检查：

- 类型定义是否同步
- API 返回字段是否与前端类型一致
- 新增路由或组件是否引用了不存在的模块

### Bot 失败

```bash
cd services/bot
pip install -r requirements.txt
python -m py_compile main.py
```

优先检查：

- 新增 import 是否正确
- 配置项是否在 `app/config.py` 中声明
- 调用 API 的字段名是否与后端一致

## 运行期冒烟失败

### API `/health` 不通

- 服务没起
- 端口没映射
- 数据库连接失败导致启动退出

先看：

```bash
docker compose ps
docker compose logs --tail=200 ember-api
```

### Bot `/health` 不通

- Bot 容器未启动
- `TELEGRAM_BOT_TOKEN` / `INTERNAL_API_SECRET` 缺失
- Webhook 相关配置错误导致启动失败

先看：

```bash
docker compose logs --tail=200 ember-bot
```

## 集成链路问题

### Emby 相关功能失败

优先检查：

- `EMBY_URL`
- `EMBY_API_KEY`

如果是用户可见行为异常，不要只看前端，直接看 API 日志。

### TMDB / 追剧日历失败

优先检查：

- `TMDB_API_KEY` 是否存在
- 返回的是明确错误，还是代码把错误吞了

### Telegram 本地联调不通

不要在这里反复猜，直接去：

- [Cloudflared 本地联调](./cloudflared-local-testing.md)

### Bot 内部 API 返回 401

优先检查：

- API 与 Bot 的 `INTERNAL_API_SECRET` 是否完全一致
- 请求头是否真的带了 `X-Internal-Secret`

## 什么时候该停手

出现下面任一情况，就别继续堆命令输出了，先修配置：

- 环境变量仍是示例值
- Webhook 地址不是公网地址
- Bot 和 API 用了不同的 `INTERNAL_API_SECRET`
- 依赖的第三方服务本身不可达

## 相关文档

- [测试指南](./testing.md)
- [手工测试清单](./manual-testing-checklist.md)
- [部署排障](./deployment-troubleshooting.md)
