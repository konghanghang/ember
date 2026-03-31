# P0-2: 客户端设备管理（Client Device Management）

## 功能描述

提供设备视角的运维能力：
- 客户端黑名单（按客户端名）
- 一键强制注销违规设备
- 活跃设备与客户端分布统计

该方案优先复用 Ember 现有 `EmbyService.GetSessions()` 能力，避免重复造轮子。

**优先级**：P0
**状态**：✅ 已完成（2026-03-05）

---

## Ember 对齐要点

1. API 路径统一为 `/api/v1/admin/devices/*`
2. `userId` 参数类型为 `string`
3. 设备信息主来源是 Emby `/emby/Devices` + `/emby/Sessions`
4. 黑名单优先按 `clientName` 管理（与 EmbyPulse 实际实现一致）
5. 为避免静态/参数同层冲突，设备注销路由使用静态前缀（`/devices/logout/:deviceId`）

---

## 数据模型设计

### 1. 客户端黑名单表 `client_blacklists`

```go
type ClientBlacklist struct {
    ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    ClientName string   `json:"clientName" gorm:"column:clientName;size:100;uniqueIndex;not null"`
    NormalizedClientName string `json:"-" gorm:"column:normalizedClientName;size:100;uniqueIndex;not null"` // lower(trim(clientName))
    Reason    string    `json:"reason,omitempty" gorm:"column:reason;size:255"`
    CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

### 2. 设备操作日志表 `device_actions`

```go
type DeviceAction struct {
    ID         string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    DeviceID   string    `json:"deviceId" gorm:"column:deviceId;size:100;index"`
    UserID     string    `json:"userId,omitempty" gorm:"column:userId;size:25;index"`
    ClientName string    `json:"clientName" gorm:"column:clientName;size:100"`
    Action     string    `json:"action" gorm:"column:action;size:50;not null"` // blacklist/unblacklist/logout
    Note       string    `json:"note,omitempty" gorm:"column:note;size:255"`
    CreatedAt  time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

---

## API 端点设计

### 1. 设备列表

`GET /api/v1/admin/devices`

Query:
- `userId`（可选，string）
- `clientName`（可选）
- `isBlacklisted`（可选）
- `page` / `pageSize`

### 2. 黑名单列表

`GET /api/v1/admin/devices/blacklist`

### 3. 添加黑名单

`POST /api/v1/admin/devices/blacklist`

```json
{
  "clientName": "Infuse",
  "reason": "共享账号风险"
}
```

### 4. 移除黑名单

`DELETE /api/v1/admin/devices/blacklist/:clientName`

### 5. 强制注销单设备

`POST /api/v1/admin/devices/logout/:deviceId`

### 6. 执行黑名单批量注销

`POST /api/v1/admin/devices/blacklist/logout-all`

### 7. 统计信息

`GET /api/v1/admin/devices/stats`

Response:

```json
{
  "data": {
    "clientDistribution": [
      { "clientName": "Emby Web", "count": 25 }
    ],
    "topDevices": [
      { "deviceName": "iPhone 13", "count": 10 }
    ],
    "blacklistedClientCount": 3,
    "activeSessionCount": 6
  }
}
```

---

## 核心实现建议

### 服务层建议

```go
func (s *DeviceService) GetDevices(ctx context.Context, req GetDevicesRequest) (*DeviceListResponse, error)
func (s *DeviceService) AddClientToBlacklist(ctx context.Context, clientName, reason string) error
func (s *DeviceService) RemoveClientFromBlacklist(ctx context.Context, clientName string) error
func (s *DeviceService) LogoutDevice(ctx context.Context, deviceID string) error
func (s *DeviceService) LogoutBlacklistedDevices(ctx context.Context) (int, error)
func (s *DeviceService) GetStats(ctx context.Context) (*DeviceStats, error)
```

### 关键行为说明

1. 设备状态判定优先使用 `Sessions`（当前播放）
2. 黑名单命中规则使用 `clientName`（忽略大小写）
3. 批量注销调用 Emby 设备删除/踢下线接口
4. 操作日志写入 `device_actions` 便于审计

---

## 前端页面设计

### 路由

```ts
{
  path: '/console/devices',
  name: 'console-devices',
  meta: { requiresAuth: true, role: 'admin' },
  component: () => import('@/views/admin/DevicesView.vue')
}
```

### 页面模块

- 统计卡片
- 黑名单管理
- 设备列表（支持一键注销）
- 最近操作日志

---

## 验证清单

- [x] 黑名单新增/删除可用
- [x] 批量注销只影响黑名单客户端
- [x] 统计图与列表数据口径一致
- [x] Emby 不可达时返回清晰错误

**预计工作量**：4-6 天
