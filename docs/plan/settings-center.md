# Ember 设置中心与运行期配置治理方案

## Context

当前 Ember 的系统配置已经分裂成两套来源：

1. **数据库 `settings` 表**
   - 当前只承载少量业务开关，如 `registration_mode`、`default_trial_days`、`notify_group_link`、`email_verification`、`stripe_allowed_payment_methods`
   - 管理后台页面 [services/web/src/views/admin/SettingsView.vue](../../services/web/src/views/admin/SettingsView.vue) 只覆盖这部分配置

2. **Docker / 进程环境变量**
   - API、Bot、支付、邮件、Emby、TMDB、MoviePilot、cron 等核心能力仍通过 `os.Getenv()` 或 `os.environ[]` 读取
   - 典型位置：
     - [services/api/cmd/server/main.go](../../services/api/cmd/server/main.go)
     - [services/api/internal/integrations/emby/emby.go](../../services/api/internal/integrations/emby/emby.go)
     - [services/api/internal/services/email.go](../../services/api/internal/services/email.go)
     - [services/api/internal/services/payment/service.go](../../services/api/internal/services/payment/service.go)
     - [services/bot/app/config.py](../../services/bot/app/config.py)

这已经带来了三个实际问题：

### 1. 配置入口分裂

管理员在 Web 后台只能改一小部分配置；大量运行期配置仍要去改 `.env` 或 Docker Compose，再重启容器。对于 Emby、SMTP、TMDB、MoviePilot 这类运营期会真实调整的集成配置，这种方式已经过时。

### 2. 语义和校验开始漂移

当前设置页文案写着“默认试用天数设为 0 则无试用”，但后端 `SettingService` 实际拒绝 `0`：

- 前端：[services/web/src/views/admin/SettingsView.vue#L232](../../services/web/src/views/admin/SettingsView.vue#L232)
- 后端：[services/api/internal/handlers/setting.go](../../services/api/internal/handlers/setting.go)

这说明前后端没有共享同一份配置定义，继续在现有 `key/value` 白名单上堆逻辑，只会让这种漂移越来越多。

### 3. 配置没有明确分层

现在“业务开关”“第三方集成参数”“部署密钥”都被笼统叫做配置，但它们的性质完全不同：

- 有些应该支持后台在线修改并立即生效
- 有些可以在线修改，但必须测试连接、保护敏感值
- 有些根本不应该放进可编辑设置页，例如 `DATABASE_URL`、`JWT_SECRET`

如果不先分层，所谓“设置中心”最后只会变成一个更大的垃圾抽屉。

---

## 目标与非目标

### 目标

1. 建立一个统一的**设置中心**，让管理员通过后台完成大多数运行期配置
2. 保持现有 Docker / `.env` 部署完全兼容，不破坏用户可见行为
3. 为前后端提供同一份配置定义，消除文案、校验、生效行为的漂移
4. 对敏感信息提供最基本的安全边界：不明文回显，支持加密存储
5. 为后续支付、通知、任务调度等配置迁移提供统一基础设施

### 非目标

1. 不做多环境配置（dev / staging / prod 同页切换）
2. 不做配置历史版本回滚
3. 不做通用审计系统，只保留基础“谁修改了配置”的能力
4. 不把数据库连接、JWT 密钥、内部鉴权密钥等部署边界配置做成在线可编辑
5. 不在本期统一改造全部历史 env 项，必须分阶段迁移

---

## 核心设计决策

## 1. 配置分三层

### A. 运行期业务配置

特点：

- 与业务策略直接相关
- 改完应立即生效
- 不需要重启服务
- 大多不敏感

典型项：

- `registration_mode`
- `default_trial_days`
- `email_verification`
- `notify_group_link`
- `stripe_allowed_payment_methods`

### B. 运行期集成配置

特点：

- 第三方服务地址、账号、开关、限额等
- 允许通过后台编辑
- 需要配置状态检查或“测试连接”
- 部分字段属于敏感信息，必须加密存储并禁止明文回显

典型项：

- `EMBY_URL`
- `EMBY_API_KEY`
- `NEXT_PUBLIC_EMBY_URL`
- `TMDB_API_KEY`
- `MOVIEPILOT_URL`
- `MOVIEPILOT_USERNAME`
- `MOVIEPILOT_PASSWORD`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `EMAIL_CODE_EXPIRY_MINUTES`
- `EMAIL_CODE_DAILY_LIMIT`
- `EMAIL_CODE_IP_DAILY_LIMIT`
- `BOT_NOTIFY_URL`

### C. 部署期 / 安全边界配置

特点：

- 决定进程启动、鉴权边界或基础设施连接
- 修改后通常需要重启，甚至会影响整个部署拓扑
- 不应在 Web 管理后台暴露成“可随便编辑”的普通设置

典型项：

- `DATABASE_URL`
- `JWT_SECRET`
- `INTERNAL_API_SECRET`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`
- `PORT`
- `AUTO_MIGRATE`

这些配置只在设置中心展示**状态和来源**，不允许在线编辑。

## 2. 读取策略按配置分层控制

兼容阶段可以采用“数据库覆盖值 > 环境变量 > 默认值”，但不能把它当成永恒真理。

原因很简单：

1. 对已经迁移完成、明确由设置中心托管的运行期集成配置，继续偷偷回退 env，只会制造行为不透明
2. 对部署期 / 安全边界配置，保留 env 读取仍然合理
3. 对有代码默认值的项，只有数据库和 env 都没有时才应该回退默认值

因此实际规则应该是：

```text
托管运行期配置：
- settings 有值 -> 用 settings
- settings 无值 -> 视为未配置或回退代码默认值

部署边界配置：
- settings 无权接管
- 继续读取 env / default
```

## 3. 配置定义由代码维护，不由数据库维护

数据库只负责存“值”，不负责存“配置元数据”。

原因很简单：

1. 分组、标签、描述、校验规则本质上是代码契约
2. 如果把元数据也塞进数据库，只会制造两套事实来源
3. 当前 `settings` 漂移问题本来就是因为没有统一定义

因此需要引入统一的 `ConfigDefinition` 注册表，由后端代码维护。

## 4. 敏感值必须加密存储，且不允许明文回显

如果把 `SMTP_PASSWORD`、`EMBY_API_KEY`、`STRIPE_SECRET_KEY` 这种值从 env 迁到数据库，却仍然明文存储，那根本不是进步，只是把明文从 `.env` 挪到数据库里。

最低要求：

1. 敏感配置项在数据库中以密文保存
2. 应用启动时从环境变量读取一个**配置加密主密钥**
3. API 响应不返回敏感值明文，只返回：
   - `hasValue`
   - `source`
   - `sensitive`
   - `restartRequired`
4. 管理员只有在输入新值时才会覆盖旧值

## 5. 设置中心必须按配置域分组，不允许继续做单页大表单

当前 [services/web/src/views/admin/SettingsView.vue](../../services/web/src/views/admin/SettingsView.vue) 是“一个页面 + 一个表单 + 一个保存按钮”的形态，随着配置增长必然失控。

新设置中心必须：

1. 按配置域组织内容
2. 每组独立保存、独立校验、独立反馈
3. 每组需要时提供“测试连接”
4. 明确展示每一项的来源和生效方式

---

## 当前配置现状盘点

下表基于当前代码中 env 读取和 `settings` 表使用情况整理。

| 配置项 | 当前来源 | 当前用途 | 目标来源 | 敏感 | 在线编辑 | 需重启 |
|--------|----------|----------|----------|------|----------|--------|
| `registration_mode` | `settings` | 注册模式 | `settings` | 否 | 是 | 否 |
| `default_trial_days` | `settings` | 默认试用天数 | `settings` | 否 | 是 | 否 |
| `notify_group_link` | `settings` | Telegram 欢迎消息链接 | `settings` | 否 | 是 | 否 |
| `email_verification` | `settings` | 注册邮箱验证业务开关 | `settings` | 否 | 是 | 否 |
| `stripe_allowed_payment_methods` | `settings` | Stripe 支付方式白名单 | `settings` | 否 | 是 | 否 |
| `EMBY_URL` | env | Emby API 地址 | `settings/env` | 否 | 是 | 否 |
| `EMBY_API_KEY` | env | Emby API 鉴权 | `settings/env` | 是 | 是 | 否 |
| `NEXT_PUBLIC_EMBY_URL` | env | 前端展示用 Emby 地址 | `settings/env` | 否 | 是 | 否 |
| `TMDB_API_KEY` | env | TMDB 搜索/追剧日历 | `settings/env` | 是 | 是 | 否 |
| `MOVIEPILOT_URL` | env | MoviePilot API 地址 | `settings/env` | 否 | 是 | 否 |
| `MOVIEPILOT_USERNAME` | env | MoviePilot 用户名 | `settings/env` | 是 | 是 | 否 |
| `MOVIEPILOT_PASSWORD` | env | MoviePilot 密码 | `settings/env` | 是 | 是 | 否 |
| `SMTP_HOST` | env | SMTP 主机 | `settings/env` | 否 | 是 | 否 |
| `SMTP_PORT` | env | SMTP 端口 | `settings/env` | 否 | 是 | 否 |
| `SMTP_USERNAME` | env | SMTP 用户名 | `settings/env` | 是 | 是 | 否 |
| `SMTP_PASSWORD` | env | SMTP 密码 | `settings/env` | 是 | 是 | 否 |
| `SMTP_FROM` | env | SMTP 发件人 | `settings/env` | 否 | 是 | 否 |
| `EMAIL_CODE_EXPIRY_MINUTES` | env | 邮箱验证码过期时间 | `settings/env` | 否 | 是 | 否 |
| `EMAIL_CODE_DAILY_LIMIT` | env | 每邮箱发送上限 | `settings/env` | 否 | 是 | 否 |
| `EMAIL_CODE_IP_DAILY_LIMIT` | env | 每 IP 发送上限 | `settings/env` | 否 | 是 | 否 |
| `BOT_NOTIFY_URL` | env | API 推送到 Bot 的地址 | `settings/env` | 否 | 是 | 否 |
| `CRON_ENABLED` | env | cron 总开关 | `settings` | 否 | 是 | 是 |
| `CRON_SCHEDULE` | env | 过期检查 cron 表达式 | `settings` | 否 | 是 | 是 |
| `CRON_TIMEZONE` | env | cron 时区 | `settings` | 否 | 是 | 是 |
| `RANKING_CRON_ENABLED` | env | 排行榜 cron 开关 | `settings` | 否 | 是 | 是 |
| `RANKING_DAILY_SCHEDULE` | env | 日榜 cron 表达式 | `settings` | 否 | 是 | 是 |
| `RANKING_WEEKLY_SCHEDULE` | env | 周榜 cron 表达式 | `settings` | 否 | 是 | 是 |
| `TV_CALENDAR_SYNC_SCHEDULE` | env | 追剧日历同步 cron 表达式 | `settings` | 否 | 是 | 是 |
| `STRIPE_SECRET_KEY` | env | Stripe Secret Key | `settings/env` | 是 | 是 | 否 |
| `STRIPE_WEBHOOK_SECRET` | env | Stripe Webhook Secret | 只读 env | 是 | 否 | 是 |
| `STRIPE_SUCCESS_URL` | env | 支付成功跳转 URL | `settings/env` | 否 | 是 | 否 |
| `STRIPE_CANCEL_URL` | env | 支付取消跳转 URL | `settings/env` | 否 | 是 | 否 |
| `DATABASE_URL` | env | 数据库连接串 | env | 是 | 否 | 是 |
| `JWT_SECRET` | env | JWT 签名密钥 | env | 是 | 否 | 是 |
| `INTERNAL_API_SECRET` | env | API/Bot 内部鉴权 | env | 是 | 否 | 是 |
| `ADMIN_USERNAME` | env | 首次管理员初始化 | env | 否 | 否 | 是 |
| `ADMIN_PASSWORD` | env | 首次管理员初始化密码 | env | 是 | 否 | 是 |
| `TELEGRAM_BOT_TOKEN` | env | Bot Token | env | 是 | 否 | 是 |
| `TELEGRAM_ADMIN_CHAT_ID` | env | Telegram 管理员通知对象 | `settings/env` | 否 | 是 | 否 |
| `TELEGRAM_GROUP_CHAT_ID` | env | 排行榜群推送对象 | `settings/env` | 否 | 是 | 否 |
| `TELEGRAM_WEBHOOK_SECRET` | env | Telegram Webhook 校验密钥 | env | 是 | 否 | 是 |
| `WEBHOOK_URL` | env | Telegram Webhook URL | env | 是 | 否 | 是 |
| `PORT` | env | API 监听端口 | env | 否 | 否 | 是 |
| `AUTO_MIGRATE` | env | 启动自动迁移 | env | 否 | 否 | 是 |

---

## v1 实施范围

v1 只解决最真实、最常改、最适合在线管理的配置，不追求一步吞掉全部配置。

### v1 迁移到设置中心的配置

#### 基础业务

- `registration_mode`
- `default_trial_days`
- `notify_group_link`
- `email_verification`
- `stripe_allowed_payment_methods`

#### 媒体集成

- `EMBY_URL`
- `EMBY_API_KEY`
- `NEXT_PUBLIC_EMBY_URL`
- `TMDB_API_KEY`
- `MOVIEPILOT_URL`
- `MOVIEPILOT_USERNAME`
- `MOVIEPILOT_PASSWORD`

#### 邮件服务

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `EMAIL_CODE_EXPIRY_MINUTES`
- `EMAIL_CODE_DAILY_LIMIT`
- `EMAIL_CODE_IP_DAILY_LIMIT`

#### 通知

- `BOT_NOTIFY_URL`

#### 支付

- `STRIPE_SECRET_KEY`
- `STRIPE_SUCCESS_URL`
- `STRIPE_CANCEL_URL`

#### 任务调度

- `CRON_ENABLED`
- `CRON_SCHEDULE`
- `CRON_TIMEZONE`
- `RANKING_CRON_ENABLED`
- `RANKING_DAILY_SCHEDULE`
- `RANKING_WEEKLY_SCHEDULE`
- `TV_CALENDAR_SYNC_SCHEDULE`

#### Telegram 运行期通知

- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_GROUP_CHAT_ID`

### v1 保持只读展示的配置

- `DATABASE_URL`
- `JWT_SECRET`
- `INTERNAL_API_SECRET`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`
- `PORT`
- `AUTO_MIGRATE`

### v1 暂不改造的配置

- `STRIPE_WEBHOOK_SECRET`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`

原因：

1. 这些项直接决定 Webhook 校验和 Bot/API 启动边界
2. 它们属于更明确的部署期安全边界，而不是普通运行期配置
3. 把它们继续留在 env 更符合“先收运行期，再守住边界”的原则

---

## 后端设计

## 1. 引入统一配置定义注册表

新增统一配置定义结构：

```go
type ConfigValueType string

const (
	ConfigValueString   ConfigValueType = "string"
	ConfigValueSecret   ConfigValueType = "secret"
	ConfigValueBoolean  ConfigValueType = "boolean"
	ConfigValueInteger  ConfigValueType = "integer"
	ConfigValueURL      ConfigValueType = "url"
	ConfigValueEnum     ConfigValueType = "enum"
	ConfigValueJSONList ConfigValueType = "json_list"
)

type ConfigDefinition struct {
	Key             string
	Group           string
	Label           string
	Description     string
	Type            ConfigValueType
	DefaultValue    string
	Placeholder     string
	Editable        bool
	Sensitive       bool
	RestartRequired bool
	Options         []ConfigOption
	Validate        func(string) error
}
```

说明：

1. `ConfigDefinition` 是系统的唯一配置元数据来源
2. 现有 `SettingService` 的硬编码白名单逻辑要逐步被注册表替代
3. 前端展示和后端校验都从这里派生

## 2. 继续复用 `settings` 表，但补足元数据

当前 `settings` 模型定义很薄：

- [services/api/internal/models/setting.go](../../services/api/internal/models/setting.go)

v1 需要把它扩展为：

```go
type Setting struct {
	Key             string    `gorm:"column:key;type:varchar(100);primaryKey"`
	Value           string    `gorm:"column:value;type:text;not null"`
	IsEncrypted     bool      `gorm:"column:isEncrypted;default:false;not null"`
	UpdatedByUserID *string   `gorm:"column:updatedByUserId;size:25"`
	UpdatedAt       time.Time `gorm:"column:updatedAt;autoUpdateTime"`
}
```

设计要求：

1. `Value` 不能继续限制在 500 字符，避免密文、JSON 和长 URL 被截断
2. `IsEncrypted` 标识该条记录是否是密文
3. `UpdatedByUserID` 只记录最后修改者，不扩展成完整审计系统

## 3. 新增统一读取层

新增配置读取服务，例如：

```go
type ConfigResolver interface {
	Get(key string) ResolvedConfigValue
	List() []ResolvedConfigValue
}

type ResolvedConfigValue struct {
	Key             string `json:"key"`
	Value           string `json:"value,omitempty"`
	HasValue        bool   `json:"hasValue"`
	Source          string `json:"source"` // database | env | default | unset
	Editable        bool   `json:"editable"`
	Sensitive       bool   `json:"sensitive"`
	RestartRequired bool   `json:"restartRequired"`
	IsEncrypted     bool   `json:"isEncrypted"`
}
```

解析规则：

1. 查配置定义
2. 查 `settings`
3. 若未命中，再读对应 env
4. 若仍为空，再回退默认值
5. `secret` 类型不返回 `Value` 明文，仅返回 `HasValue=true`

## 4. 敏感值加密策略

新增环境变量：

```text
CONFIG_ENCRYPTION_KEY=
```

规则：

1. 只有 `Sensitive=true` 的配置会使用该密钥加密存储
2. 没有配置 `CONFIG_ENCRYPTION_KEY` 时，禁止把新的敏感配置写入数据库
3. 已有 env 仍可只读展示，不影响现有部署
4. 响应层不暴露敏感值明文

推荐实现：

- 使用 AES-GCM
- 密文格式可采用 `base64(nonce + ciphertext)`
- 解密失败时直接把该配置标记为错误状态，不静默回退

## 5. API 设计

当前后台以配置中心接口为主，旧 `/api/v1/admin/settings` 已下线：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/configs` | 获取全部配置定义 + 当前解析结果 |
| PATCH | `/api/v1/admin/configs/:key` | 更新单个配置 |
| POST | `/api/v1/admin/configs/:group/test` | 测试某个配置组的连接状态 |
| POST | `/api/v1/admin/configs/import-env` | 可选：将 env 当前值导入数据库 |

### 5.1 `GET /api/v1/admin/configs`

响应结构：

```json
{
  "data": [
    {
      "key": "EMBY_URL",
      "group": "media",
      "label": "Emby 服务地址",
      "description": "后端访问 Emby API 的基础地址",
      "type": "url",
      "editable": true,
      "sensitive": false,
      "restartRequired": false,
      "source": "env",
      "hasValue": true,
      "value": "https://emby.example.com",
      "placeholder": "https://your-emby-server.com"
    },
    {
      "key": "EMBY_API_KEY",
      "group": "media",
      "label": "Emby API Key",
      "description": "用于访问 Emby API 的鉴权密钥",
      "type": "secret",
      "editable": true,
      "sensitive": true,
      "restartRequired": false,
      "source": "env",
      "hasValue": true
    }
  ]
}
```

规则：

1. 非敏感项可以回传实际值
2. 敏感项只返回 `hasValue`
3. 如果项来自 env，前端必须明确显示“当前来源：环境变量”

### 5.2 `PATCH /api/v1/admin/configs/:key`

请求结构：

```json
{
  "value": "https://emby.example.com"
}
```

敏感字段更新行为：

1. 空字符串不表示“清空”，而表示“未修改”
2. 若要支持清空，必须显式传 `clear=true`

推荐请求结构：

```json
{
  "value": "new-secret",
  "clear": false
}
```

响应返回该项的最新解析状态，而不是数据库原始行。

### 5.3 `POST /api/v1/admin/configs/:group/test`

v1 需要支持的测试组：

- `media`
  - 测试 Emby 连接
  - 若配置了 MoviePilot，同时测试 MoviePilot 可达性
- `email`
  - 验证 SMTP 参数完整性
  - 发起连接级检查，不实际发送验证码

返回结构：

```json
{
  "success": true,
  "message": "Emby 连接成功",
  "details": [
    {
      "target": "emby",
      "success": true,
      "message": "连接成功"
    }
  ]
}
```

### 5.4 `POST /api/v1/admin/configs/import-env`

这是迁移辅助接口，不是用户高频操作。

行为：

1. 遍历允许导入的配置定义
2. 如果该 key 当前没有数据库值，但 env 有值，则写入数据库
3. 敏感项按密文写入
4. 返回导入成功/跳过/失败明细

---

## 前端设置中心设计

## 1. 页面定位

当前 [services/web/src/views/admin/SettingsView.vue](../../services/web/src/views/admin/SettingsView.vue) 需要从“系统设置单页表单”升级为“设置中心”。

设计原则参考 `ui-ux-pro-max` 的结论：扁平、直接、状态优先，不做花哨布局。

### UI 风格要求

1. 维持现有 Ember 后台的浅色、卡片化风格
2. 避免继续叠加大面积说明文字
3. 每个配置项的状态必须显性展示，而不是藏在弹窗提示里

## 2. 信息架构

页面固定分为两部分：

### 顶部概览区

展示 4 张状态卡片：

1. 已配置项数量
2. 缺失项数量
3. 敏感项已设置数量
4. 需重启项数量

### 主体区

左侧：分组导航

- 基础业务
- 媒体集成
- 邮件服务
- 支付
- 通知与 Bot
- 任务调度
- 部署与密钥

右侧：当前分组配置卡片

## 3. 分组定义

### 基础业务

- `registration_mode`
- `default_trial_days`
- `email_verification`
- `notify_group_link`
- `stripe_allowed_payment_methods`

### 媒体集成

- `EMBY_URL`
- `EMBY_API_KEY`
- `NEXT_PUBLIC_EMBY_URL`
- `TMDB_API_KEY`
- `MOVIEPILOT_URL`
- `MOVIEPILOT_USERNAME`
- `MOVIEPILOT_PASSWORD`

### 邮件服务

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `EMAIL_CODE_EXPIRY_MINUTES`
- `EMAIL_CODE_DAILY_LIMIT`
- `EMAIL_CODE_IP_DAILY_LIMIT`

### 支付

- `stripe_allowed_payment_methods`
- `STRIPE_SECRET_KEY`
- `STRIPE_SUCCESS_URL`
- `STRIPE_CANCEL_URL`
- `STRIPE_WEBHOOK_SECRET`（只读）

### 通知与 Bot

- `BOT_NOTIFY_URL`
- `notify_group_link`
- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_GROUP_CHAT_ID`
- 其他 Telegram 启动边界配置暂时只读

### 任务调度

支持在线编辑，但明确标记为“保存后需重启 API 才生效”。

### 部署与密钥

全部只读展示：

- `DATABASE_URL`
- `JWT_SECRET`
- `INTERNAL_API_SECRET`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`
- `PORT`
- `AUTO_MIGRATE`

## 4. 单项配置展示结构

每个配置项卡片必须显示：

1. 标题
2. 描述
3. 输入控件
4. 来源标签：`数据库` / `环境变量` / `默认值` / `未设置`
5. 生效方式标签：`立即生效` / `需重启`
6. 对只读边界项明确显示“为什么只读”和“缺失会造成什么影响”
6. 敏感状态：`已设置` / `未设置`

示意：

```text
Emby API Key
用于访问 Emby API 的鉴权密钥
[ 已设置 ] [ 来源: 环境变量 ] [ 立即生效 ]
[ 输入新值进行覆盖 ]
```

## 5. 分组级交互

每组统一提供：

- `保存`
- `重置未保存`
- `测试连接`（仅对需要的组显示）

反馈规则：

1. 保存过程中按钮进入 loading 状态
2. 保存成功后给出组级成功反馈
3. 测试连接返回具体失败目标，不允许只提示“失败”

## 6. 敏感字段交互

敏感字段必须遵守：

1. 不回显旧值
2. 输入框为空时表示不修改
3. 提供“清空当前值”单独操作，而不是用空字符串语义偷懒
4. 显示当前值来源和是否已设置

## 7. 只读配置展示

只读配置不应该像普通表单一样展示空输入框。

推荐展示为状态列表：

```text
JWT_SECRET
- 来源：环境变量
- 状态：已设置
- 可编辑：否
- 原因：部署期安全边界配置
```

---

## 校验与行为约束

## 1. 通用约束

1. 任何配置写入前都必须经过 `ConfigDefinition.Validate`
2. 不允许前端自己维护与后端不一致的业务规则
3. 前端控件限制只是体验优化，真正的校验以服务端为准

## 2. v1 关键校验

### `default_trial_days`

必须先统一语义。

建议结论：

- **允许 `0`**
- `0` 表示“无试用”

理由：

1. 前端已经按这个语义写了文案
2. 这比“必须大于 0，但页面还说可设 0”更干净
3. 业务上“开放注册但无试用”是合理场景

对应地，后端校验应从 `days <= 0` 改为 `days < 0`。

### `registration_mode`

仅允许：

- `open`
- `invite`

### `email_verification`

仅允许：

- `true`
- `false`

并且展示层要明确：

```text
实际生效 = email_verification == true AND SMTP 已完整配置
```

### `EMBY_URL`

要求：

1. 必须是合法 URL
2. 不自动补尾部 `/`
3. 保存时统一去掉尾部 `/`

### `SMTP_PORT`

要求：

1. 必须是 1 到 65535 的整数
2. 可默认 `587`

### `stripe_allowed_payment_methods`

仅允许：

- `card`
- `alipay`
- `wechat_pay`

空数组非法；若表示“跟随 Stripe Dashboard”，应使用空字符串或显式的 `mode=dynamic` 表达，而不是把空数组当成多义值。

---

## 实现阶段与迁移策略

## 阶段 1：统一配置定义与读取层

目标：

1. 引入 `ConfigDefinition`
2. 引入 `ConfigResolver`
3. 先不大改现有页面
4. 业务侧从直接 `os.Getenv()` 逐步切到统一读取层

要求：

1. 老 env 行为完全兼容
2. 任何一个配置项迁移后，不影响未迁移配置项

## 阶段 2：扩展 `settings` 存储能力

目标：

1. 扩展 `settings` 模型字段
2. 支持敏感值密文存储
3. 支持“最后修改者”记录

要求：

1. 老数据自动兼容
2. 无需手工清洗旧 `settings`

## 阶段 3：设置中心 API

目标：

1. 新增 `/admin/configs` 系列接口
2. 只让设置中心消费新接口

## 阶段 4：前端设置中心改造

目标：

1. 重写后台设置页信息架构
2. 接入分组保存和连接测试
3. 接入来源标签、生效方式标签、敏感状态

## 阶段 5：后续扩展

后续可考虑再迁：

1. cron 配置
2. Stripe Secret / URL 策略
3. Telegram 群组推送配置
4. 配置导入导出
5. 更完整的审计日志

---

## 风险与注意事项

## 1. 不能把部署边界配置随便网页化

以下项即使技术上能写进数据库，也不应该在 v1 提供在线编辑：

- `DATABASE_URL`
- `JWT_SECRET`
- `INTERNAL_API_SECRET`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`

原因不是“做不到”，而是“做了会把安全边界搞烂”。

## 2. 不能出现“保存成功但实际不生效”的伪配置

对每个配置项，必须明确告诉管理员：

1. 当前来源是什么
2. 保存后是否立即生效
3. 是否需要重启服务

否则设置中心只是换了一层 UI，问题没有解决。

## 3. 不能继续维护两份校验规则

当前 `default_trial_days` 冲突就是典型反面案例。

后续实现必须保证：

1. 后端注册表定义校验
2. 前端从接口或共享契约派生展示和输入限制

## 4. 敏感值覆盖语义必须清晰

禁止下面这种糟糕设计：

```text
输入框为空，到底表示：
- 未修改
- 置空
- 使用默认值
```

这三件事必须是三种明确动作。

---

## 测试与验收

## 1. 后端测试

### 读取优先级

1. `settings` 有值、env 有值时，返回 `settings`
2. `settings` 无值、env 有值时，返回 env
3. 两者都无值时，返回默认值或 `unset`

### 敏感值

1. 写入敏感配置后数据库中为密文
2. `GET /admin/configs` 不返回敏感值明文
3. `CONFIG_ENCRYPTION_KEY` 缺失时，敏感值写入被拒绝

### 校验

1. `default_trial_days = -1` 被拒绝
2. `default_trial_days = 0` 被接受
3. 非法 `registration_mode` 被拒绝
4. 非法 `SMTP_PORT` 被拒绝
5. 非法 `EMBY_URL` 被拒绝

### 连接测试

1. Emby 地址错误时返回可读错误
2. SMTP 参数不完整时返回明确错误
3. MoviePilot 未配置时测试结果应标明“未配置”，而不是笼统失败

### 兼容性

1. 没有任何数据库配置时，老 `.env` 部署行为不变
2. 导入 env 后，对应功能仍按原逻辑工作

## 2. 前端验收

1. 页面能按组正确展示配置
2. 每项能显示来源标签和生效方式标签
3. 敏感项不回显明文
4. 组级保存有 loading、成功、失败反馈
5. 连接测试结果可读
6. 只读配置不显示可编辑输入框

## 3. 联调验收

1. 修改 `EMBY_URL` 和 `EMBY_API_KEY` 后可立即测试连接
2. 修改 SMTP 配置后，`email_verification` 的可用状态联动正确
3. `notify_group_link` 继续保持“后台保存后立即生效”的既有行为
4. env 接管状态下页面能明确展示“当前来源：环境变量”

---

## 最终结论

这次改造的本质不是“给设置页多加几个输入框”，而是把 Ember 的配置系统从“数据库配置 + 环境变量补丁”的分裂状态，收敛成一个有边界、有优先级、有状态反馈的统一配置体系。

v1 必须克制：

1. 先把最常改、最适合在线管理的运行期配置收进设置中心
2. 保住现有 Docker / `.env` 部署兼容性
3. 守住敏感信息和部署边界，不做看起来方便、实际上很危险的在线编辑

只要这三点做对，后面的 cron、支付、Bot 配置迁移都只是增量工作；如果这三点做错，设置中心只会把现在的混乱放大。

---

## 当前实施进度（2026-03-13）

### 已完成

1. 后端已落地统一配置服务：
   - 新增 `ConfigService`，提供配置定义注册表、读取优先级解析、敏感值加密存储、分组测试、环境变量导入
   - 新增管理员接口：
     - `GET /api/v1/admin/configs`
     - `PATCH /api/v1/admin/configs/:key`
     - `POST /api/v1/admin/configs/:group/test`
     - `POST /api/v1/admin/configs/import-env`

2. `settings` 表已扩展为设置中心存储层：
   - 模型新增 `isEncrypted`
   - 模型新增 `updatedByUserId`
   - `key` 扩为 `varchar(100)`
   - `value` 扩为 `text`
   - 已补手工迁移 SQL：`infrastructure/database/20260312_01_expand_settings_for_config_center.sql`

3. v1 运行期配置读取点已接入统一配置层：
   - Emby：`EMBY_URL`、`EMBY_API_KEY`
   - 媒体展示：`NEXT_PUBLIC_EMBY_URL`
   - TMDB：`TMDB_API_KEY`
   - MoviePilot：`MOVIEPILOT_URL`、`MOVIEPILOT_USERNAME`、`MOVIEPILOT_PASSWORD`
   - SMTP：`SMTP_*`、`EMAIL_CODE_*`
   - Bot 通知：`BOT_NOTIFY_URL`
   - 任务调度：`CRON_*`、`RANKING_*`、`TV_CALENDAR_SYNC_SCHEDULE`
   - 支付：`STRIPE_SECRET_KEY`、`STRIPE_SUCCESS_URL`、`STRIPE_CANCEL_URL`
   - Telegram 运行期通知：`TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`

4. 后台设置页已重写为设置中心：
   - 分组导航
   - 来源标签（数据库 / 环境变量 / 默认值 / 未设置）
   - 生效方式标签（立即生效 / 需重启）
   - 敏感项状态展示
   - 分组保存
   - 媒体/邮件分组测试
   - 环境变量导入入口
   - “移除数据库覆盖值” 与“保存为空值”语义已区分
   - 只读边界项已展示只读原因、缺失影响和高风险缺失提示

5. 兼容性与验证：
   - Internal API `/api/v1/internal/settings/:key` 现只允许读取统一配置层中已注册的非敏感 key，未知 key 直接返回 404
   - 旧 `/api/v1/admin/settings` 路由已下线
   - `default_trial_days` 已统一为允许 `0`，表示无试用
   - `services/api` 下 `go build ./...` 已通过
   - `services/api` 下设置中心相关定向测试已通过
   - `services/web` 下 `npm run test:unit` 已通过

### 未完成

当前结论：主体功能已完成，剩余工作以验收、迁移确认和上线为主。

1. 尚未在真实数据库执行迁移；生产环境需要手工执行：
   - `psql "$DATABASE_URL" -f infrastructure/database/20260312_01_expand_settings_for_config_center.sql`

2. 自动化测试仍有缺口：
   - 前端组件测试文件已存在，但当前本地环境未完成 `vitest` 组件级实际跑测验证

3. 治理层能力不在当前范围内：
   - 不追加配置变更审计
   - 不做配置导出与巡检

### 下次继续时优先做什么

1. 先确认生产库已执行 `20260312_01_expand_settings_for_config_center.sql`
2. 在可用环境里跑通前端组件测试
3. 继续补测试闭环，不再扩展治理层能力

---

## 已落地实现与原方案差异

这部分必须写清楚。计划不是许愿池，代码已经落地后，文档就要承认现实。

### 1. v1 已经从“已上线骨架”进入“待验收上线”

当前设置中心的主体能力已经具备：

1. 配置定义注册表已经存在
2. 读取优先级已经按“数据库 > env > 默认值”实现
3. 敏感值加密存储已经实现
4. 管理后台新页面和新接口已经接通

所以后续工作重点不再是扩功能，而是：

1. 把剩余测试闭环补完整
2. 完成生产迁移确认
3. 按上线清单做验收

### 2. 仍然存在的设计缺口

#### A. 敏感项已支持显式清空，测试链路也已补齐到页面交互层

当前后端接口与前端页面已经打通 `clear=true`：

1. 管理员可以输入新值覆盖敏感配置
2. 也可以显式清空数据库覆盖值，回退到 env / default

这一块的主体能力已经闭环：

1. 已补前端草稿与 payload 的纯逻辑测试
2. 已补设置中心真实组件挂载测试
3. 已补配置中心 handler 测试

#### B. “空值语义”与只读边界展示已完成一轮收口

当前实现已经不再只靠 `AllowEmpty=true` 这个布尔值硬猜，而是给配置定义补了显式空值语义：

1. `disable`：保存为空值后表示关闭能力
2. `fallback`：保存为空值后回退到另一层运行期配置
3. `inherit`：保存为空值后跟随外部服务或上游默认行为

页面也要同步区分两件事：

1. “保存为空值”是一个显式数据库状态
2. “移除数据库覆盖值”是回退到 env/default

只读边界项当前也已经补到位：

1. 显示“为什么只读”
2. 显示“缺失会造成什么影响”
3. 对关键部署边界缺失给出高风险提示

后续工作不再是重新发明语义，而是把测试闭环和上线验证补完整。

#### C. 旧 `/admin/settings` 已下线，Internal API 也不再保留 legacy fallback

这是正确方向。既然前后端都已经切到 `/admin/configs`，就不该再把旧管理接口继续挂着制造第二套入口。

当前边界已经进一步收紧：

1. Internal API 只允许读取统一配置层中已注册的非敏感 key
2. 未注册 key 直接返回 404，不再偷偷回退到旧 `settings` 原始读取
3. 新功能只允许走 `/admin/configs`

### 3. 当前实现的正确边界

现阶段保持下面这个边界是对的：

1. 运行期业务配置和集成配置进入设置中心
2. 部署期密钥和启动期配置默认只读
3. cron、Webhook、安全边界配置先只展示，不急着网页化

这不是保守，这是好品味。先把数据结构立住，再逐步扩容。

---

## 下一阶段任务拆分

下面不是泛泛而谈，而是建议直接按优先级执行的任务列表。

### P0：把当前骨架补完整

#### 任务 1：继续补自动化测试

目标：

1. 给 `ConfigService` 补优先级解析测试
2. 给敏感值加解密补更完整单元测试
3. 给 `import-env`、分组测试接口补 handler/service 测试
4. 给前端设置中心补真实组件挂载级核心交互测试

验收：

1. 已覆盖 `database > env > default > unset`
2. 已覆盖敏感值不回显与解密错误路径
3. 已覆盖 `clear=true` 相关前端草稿/按钮逻辑
4. 已覆盖 `import-env` 与分组测试接口的 handler/service 级关键验证
5. 已覆盖设置中心关键页面交互链路（保存、清空覆盖值、分组测试）

当前剩余测试缺口：

1. 组件测试文件已覆盖关键交互，但当前本地环境未完成 `vitest` 组件级实际跑测验证

#### 任务 2：继续打磨前端“清空当前覆盖值”

目标：

1. 每个数据库来源的可编辑项都提供独立“移除数据库覆盖值”动作
2. 敏感项明确支持“清空数据库覆盖值”
3. 非敏感但允许空值的项，继续区分“设为空值”和“回退来源”

验收：

1. 管理员能明确知道自己执行的是哪种动作
2. 执行后页面来源标签和状态立即刷新
3. 不再依赖“把输入框留空”这种含糊语义
4. 已补组件级测试覆盖这条交互链路

状态：已完成。当前页面已明确区分“保存为空值”和“移除数据库覆盖值”。

#### 任务 3：补运维上线说明

目标：

1. 把数据库迁移、`CONFIG_ENCRYPTION_KEY`、env 导入时机写成操作步骤
2. 明确首次启用设置中心时的推荐顺序

验收：

1. 新环境能按文档从 0 启用
2. 老环境能按文档安全迁移

状态：已完成。本文档后半段已包含上线、迁移、回滚操作清单。

### P1：收敛剩余边界项与展示能力

#### 任务 4：迁移 cron 配置

范围：

1. `CRON_ENABLED`
2. `CRON_SCHEDULE`
3. `CRON_TIMEZONE`
4. `RANKING_CRON_ENABLED`
5. `RANKING_DAILY_SCHEDULE`
6. `RANKING_WEEKLY_SCHEDULE`
7. `TV_CALENDAR_SYNC_SCHEDULE`

前提：

1. 必须先定义热更新策略
2. 明确哪些修改需要重建 cron 实例
3. UI 必须清楚提示“保存后需重启”还是“保存后可重载”

状态：已完成。当前实现为“可编辑，但需重启 API 才生效”。

#### 任务 5：收敛支付与 Telegram 剩余配置

范围：

1. Stripe Secret / 跳转 URL
2. Telegram 群组和管理员通知目标

状态：已完成（按当前边界）。

1. `STRIPE_SECRET_KEY`、`STRIPE_SUCCESS_URL`、`STRIPE_CANCEL_URL` 已接入统一配置层
2. `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID` 已接入统一配置层，Bot 运行期通过 Internal API 读取
3. `STRIPE_WEBHOOK_SECRET`、`TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`WEBHOOK_URL` 仍保持只读 env，但已补只读原因、缺失影响和高风险缺失展示

### P2：治理层能力

当前结论：不做。

原因：

1. 本项目当前阶段更需要把设置中心主体能力、测试闭环和上线迁移做扎实
2. `UpdatedByUserID` 已能满足“最后是谁改的”这个最小追踪需求
3. 配置导出、巡检和更完整审计会显著增加实现和维护复杂度，但当前收益不足

如果未来出现明确的运维痛点，再单独立项，不挂在本期设置中心收尾任务里。

---

## 上线与迁移操作清单

这一段给执行的人看，不是给写文档的人自我感动。

### 首次启用设置中心

1. 备份数据库，至少备份 `settings` 表
2. 在目标环境设置 `CONFIG_ENCRYPTION_KEY`
3. 执行迁移脚本：
   `psql "$DATABASE_URL" -f infrastructure/database/20260312_01_expand_settings_for_config_center.sql`
4. 部署包含设置中心代码的新版本 API 和 Web
5. 登录后台，先只读检查 `/admin/configs` 页面是否正常加载
6. 视需要执行一次“导入环境变量”
7. 导入后优先测试：
   - 媒体组连接测试
   - 邮件组连接测试
8. 确认关键能力正常后，再考虑清理旧 env

### 老环境迁移建议

推荐顺序：

1. 先部署新代码，不改老 env
2. 通过设置中心观察来源和缺失状态
3. 导入 env 到数据库
4. 验证关键集成
5. 最后再逐项清理不再需要的 env

原因很简单：先把行为跑通，再做收口。不要反过来先删 env，把自己逼进死角。

### 禁止事项

1. 不要在没有 `CONFIG_ENCRYPTION_KEY` 的环境里尝试保存敏感配置
2. 不要在没跑迁移脚本的生产库上直接启用新写入逻辑
3. 不要一上线就删除全部旧 env
4. 不要把部署期密钥当作普通表单配置开放编辑

---

## 回滚策略

好的计划必须包含失败后的退路。

### 1. 代码回滚

如果新版本设置中心有问题：

1. Web 可以直接回滚到旧管理页版本
2. API 可以回滚到旧版本
3. 已扩展的 `settings` 表字段可以保留，不需要回退 schema

原因：

1. 这次迁移是向后兼容扩展
2. 旧代码通常不会依赖新字段
3. 保留 schema 比折腾回滚 SQL 安全

### 2. 配置回滚

如果某项数据库覆盖值导致故障：

1. 优先使用 `clear=true` 清理数据库覆盖值
2. 让配置回退到 env
3. 若 env 本身就是错误来源，再改 env 并重启对应服务

### 3. 敏感值异常回滚

如果 `CONFIG_ENCRYPTION_KEY` 配错导致敏感值无法解密：

1. 页面和接口应暴露错误状态，不允许静默吞掉
2. 先恢复正确的 `CONFIG_ENCRYPTION_KEY`
3. 如无法恢复，再人工清理对应数据库覆盖值，回退到 env

这里绝不能做“解密失败就偷偷回退 env”的补丁。那会让错误被藏起来，最后把人坑死。

---

## 文档维护规则

这份文档以后必须按“设计文档 + 进度文档”双重角色维护，不允许再次腐烂成历史废纸。

### 1. 什么时候必须更新

出现以下任一变更时，必须同步更新本文档：

1. 新增或下线一个配置项
2. 某个配置项的来源优先级改变
3. 某个配置项从只读变为可编辑，或反过来
4. 某个配置项的“需重启/立即生效”语义改变
5. 配置分组、接口、迁移方式发生变化

### 2. 更新时至少要改哪里

1. “当前配置现状盘点”
2. “v1 实施范围”或后续阶段范围
3. “当前实施进度”
4. “下次继续时优先做什么”

### 3. 文档判断标准

每次更新后，至少要回答下面这四个问题：

1. 这个配置现在从哪里读？
2. 管理员能不能改？
3. 改完什么时候生效？
4. 出问题怎么回退？

如果文档回答不了这四个问题，这文档就是没写完。
