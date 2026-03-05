# P0-2: 客户端设备管理（Client Device Management）

## 功能描述

管理用户使用的客户端设备，支持设备黑名单、一键强制注销、实时设备监控、设备活动统计图表。帮助管理员识别异常设备（如共享账号、盗版客户端）并快速处理。

**核心价值**：
- 防止账号共享和滥用
- 识别并禁止盗版客户端
- 实时监控设备活动，快速响应异常
- 数据可视化，直观了解用户设备分布

**优先级**：P0（强烈推荐）⭐⭐⭐⭐⭐

---

## 数据模型设计

### 1. Client Device 表（client_devices）

```go
// ClientDevice 客户端设备记录
type ClientDevice struct {
    ID             uint      `gorm:"primaryKey;column:id" json:"id"`
    UserID         uint      `gorm:"column:user_id;not null;index" json:"userId"`
    DeviceID       string    `gorm:"column:device_id;size:100;not null;uniqueIndex" json:"deviceId"`
    DeviceName     string    `gorm:"column:device_name;size:255" json:"deviceName"`
    ClientName     string    `gorm:"column:client_name;size:100" json:"clientName"`
    ClientVersion  string    `gorm:"column:client_version;size:50" json:"clientVersion"`
    IsBlacklisted  bool      `gorm:"column:is_blacklisted;default:false" json:"isBlacklisted"`
    LastActiveAt   time.Time `gorm:"column:last_active_at" json:"lastActiveAt"`
    LastIP         string    `gorm:"column:last_ip;size:50" json:"lastIp"`
    CreatedAt      time.Time `gorm:"column:created_at" json:"createdAt"`
    UpdatedAt      time.Time `gorm:"column:updated_at" json:"updatedAt"`
}
```

### 2. Device Activity Log 表（device_activity_logs）

```go
// DeviceActivityLog 设备活动日志
type DeviceActivityLog struct {
    ID        uint      `gorm:"primaryKey;column:id" json:"id"`
    DeviceID  string    `gorm:"column:device_id;size:100;not null;index" json:"deviceId"`
    UserID    uint      `gorm:"column:user_id;not null;index" json:"userId"`
    Action    string    `gorm:"column:action;size:50" json:"action"` // login/logout/playback/blacklisted
    IP        string    `gorm:"column:ip;size:50" json:"ip"`
    CreatedAt time.Time `gorm:"column:created_at;index" json:"createdAt"`
}
```

---

## API 端点设计

### 1. 获取设备列表

```
GET /api/devices
Query Parameters:
  - userId: int (可选)
  - isBlacklisted: bool (可选)
  - page: int (默认 1)
  - pageSize: int (默认 20)

Response:
{
  "data": [...],
  "total": 50,
  "page": 1,
  "pageSize": 20
}
```

### 2. 添加到黑名单

```
POST /api/devices/:deviceId/blacklist
Body: { "reason": "共享账号" }
Response: { "message": "设备已加入黑名单" }
```

### 3. 从黑名单移除

```
DELETE /api/devices/:deviceId/blacklist
Response: { "message": "设备已从黑名单移除" }
```

### 4. 强制注销设备

```
POST /api/devices/:deviceId/logout
Response: { "message": "设备已强制注销" }
```

### 5. 批量强制注销黑名单设备

```
POST /api/devices/blacklist/logout-all
Response: { "message": "已注销 5 个黑名单设备" }
```

### 6. 获取设备活动统计

```
GET /api/devices/stats
Query Parameters:
  - userId: int (可选)
  - startDate: string (YYYY-MM-DD)
  - endDate: string (YYYY-MM-DD)

Response:
{
  "clientDistribution": [
    { "clientName": "Emby Web", "count": 25 },
    { "clientName": "Infuse", "count": 15 }
  ],
  "dailyActivity": [...],
  "blacklistedCount": 3,
  "totalDevices": 50
}
```

---

## 前端页面设计

### 路由配置

```typescript
{
  path: '/devices',
  name: 'DeviceManagement',
  component: () => import('@/views/devices/index.vue'),
  meta: { title: '设备管理', requiresAuth: true, requiresAdmin: true }
}
```

### 页面结构

```vue
<template>
  <div class="device-management">
    <!-- 统计卡片 -->
    <el-row :gutter="20" class="stats-cards">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-value">{{ stats.totalDevices }}</div>
            <div class="stat-label">总设备数</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-value danger">{{ stats.blacklistedCount }}</div>
            <div class="stat-label">黑名单设备</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card>
          <div ref="clientChart" style="height: 200px"></div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 工具栏 -->
    <el-row :gutter="20" class="toolbar">
      <el-col :span="8">
        <el-input v-model="searchKeyword" placeholder="搜索设备名称或用户" @input="fetchDevices" />
      </el-col>
      <el-col :span="6">
        <el-select v-model="blacklistFilter" placeholder="黑名单筛选" @change="fetchDevices">
          <el-option label="全部" :value="null" />
          <el-option label="仅黑名单" :value="true" />
          <el-option label="非黑名单" :value="false" />
        </el-select>
      </el-col>
      <el-col :span="10" style="text-align: right">
        <el-button type="danger" @click="logoutAllBlacklisted">注销所有黑名单设备</el-button>
        <el-button @click="refreshDevices">刷新</el-button>
      </el-col>
    </el-row>

    <!-- 设备列表 -->
    <el-table :data="devices" style="margin-top: 20px">
      <el-table-column prop="deviceName" label="设备名称" width="200" />
      <el-table-column prop="clientName" label="客户端" width="150" />
      <el-table-column prop="clientVersion" label="版本" width="100" />
      <el-table-column prop="user.username" label="用户" width="150" />
      <el-table-column prop="lastIp" label="最后 IP" width="150" />
      <el-table-column prop="lastActiveAt" label="最后活动" width="180">
        <template #default="{ row }">
          {{ formatTime(row.lastActiveAt) }}
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.isBlacklisted" type="danger">黑名单</el-tag>
          <el-tag v-else type="success">正常</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <el-button
            v-if="!row.isBlacklisted"
            type="warning"
            size="small"
            @click="addToBlacklist(row.deviceId)"
          >
            加入黑名单
          </el-button>
          <el-button
            v-else
            type="success"
            size="small"
            @click="removeFromBlacklist(row.deviceId)"
          >
            移出黑名单
          </el-button>
          <el-button type="danger" size="small" @click="logoutDevice(row.deviceId)">
            强制注销
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 分页 -->
    <el-pagination
      v-model:current-page="currentPage"
      v-model:page-size="pageSize"
      :total="total"
      layout="total, sizes, prev, pager, next"
      @current-change="fetchDevices"
      @size-change="fetchDevices"
    />
  </div>
</template>
```

---

## 核心逻辑实现

### 服务层关键方法

```go
// services/api/internal/services/device_service.go

// GetDevices 获取设备列表
func (s *DeviceService) GetDevices(ctx context.Context, userID *uint, isBlacklisted *bool, page, pageSize int) ([]models.ClientDevice, int64, error)

// AddToBlacklist 添加到黑名单
func (s *DeviceService) AddToBlacklist(ctx context.Context, deviceID string, reason string) error

// RemoveFromBlacklist 从黑名单移除
func (s *DeviceService) RemoveFromBlacklist(ctx context.Context, deviceID string) error

// LogoutDevice 强制注销设备
func (s *DeviceService) LogoutDevice(ctx context.Context, deviceID string) error

// LogoutAllBlacklisted 批量注销黑名单设备
func (s *DeviceService) LogoutAllBlacklisted(ctx context.Context) (int, error)

// GetStats 获取设备统计
func (s *DeviceService) GetStats(ctx context.Context, userID *uint, startDate, endDate time.Time) (map[string]interface{}, error)

// SyncDevicesFromEmby 从 Emby 同步设备信息
func (s *DeviceService) SyncDevicesFromEmby(ctx context.Context) error
```

### Emby 服务扩展

```go
// services/api/internal/services/emby_service.go

// GetSessions 获取所有活动会话
func (s *EmbyService) GetSessions(ctx context.Context) ([]EmbySession, error)

// DeleteDeviceSession 删除设备会话（强制注销）
func (s *EmbyService) DeleteDeviceSession(ctx context.Context, deviceID string) error
```

---

## 验证方式

### 1. 数据库迁移

```bash
cd services/api
go run cmd/migrate/main.go create add_device_management_tables
```

### 2. API 测试

```bash
# 获取设备列表
curl "http://localhost:8080/api/devices?page=1&pageSize=20" \
  -H "Authorization: Bearer <admin_token>"

# 添加到黑名单
curl -X POST http://localhost:8080/api/devices/abc123/blacklist \
  -H "Authorization: Bearer <admin_token>"

# 强制注销
curl -X POST http://localhost:8080/api/devices/abc123/logout \
  -H "Authorization: Bearer <admin_token>"

# 获取统计
curl "http://localhost:8080/api/devices/stats" \
  -H "Authorization: Bearer <admin_token>"
```

### 3. 前端测试

- 访问 `/devices` 页面
- 测试设备列表展示
- 测试黑名单添加/移除
- 测试强制注销功能
- 测试批量注销黑名单设备
- 验证统计图表显示

### 4. 定时同步测试

```go
// 添加定时任务（每 5 分钟同步一次）
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        deviceService.SyncDevicesFromEmby(context.Background())
    }
}()
```

---

## 实施清单

- [ ] 创建数据模型（2 个表）
- [ ] 编写数据库迁移脚本
- [ ] 实现服务层逻辑
- [ ] 扩展 Emby 服务（会话管理）
- [ ] 实现 API 端点（6 个）
- [ ] 实现前端页面
- [ ] 集成 ECharts 图表
- [ ] 实现定时同步任务
- [ ] 编写单元测试
- [ ] 更新系统架构文档

**预计工作量**：3-4 天
