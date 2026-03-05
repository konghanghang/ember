# P1-2: 播放历史查询（Playback History）

## 功能描述

分页查询播放历史（从 Emby PlaybackActivity 表），支持按用户筛选、关键词搜索，显示详细信息（播放时间、设备、客户端、时长）。

**核心价值**：
- 追踪用户观影行为
- 问题排查和审计
- 数据分析基础

**优先级**：P1（高价值功能）⭐⭐⭐⭐

---

## API 端点设计

```
GET /api/playback-history
Query Parameters:
  - userId: int (可选)
  - keyword: string (搜索媒体名称)
  - startDate: string (YYYY-MM-DD)
  - endDate: string (YYYY-MM-DD)
  - page: int
  - pageSize: int

Response:
{
  "data": [
    {
      "id": 1,
      "userId": 10,
      "username": "john_doe",
      "itemName": "Game of Thrones S03E05",
      "itemType": "Episode",
      "playedAt": "2026-03-05T20:30:00Z",
      "deviceName": "iPhone 13 Pro",
      "clientName": "Emby for iOS",
      "playDuration": 3600,
      "playDurationFormatted": "1h 0m"
    }
  ],
  "total": 500,
  "page": 1,
  "pageSize": 20
}
```

---

## 核心逻辑

```go
// GetPlaybackHistory 获取播放历史
func (s *PlaybackHistoryService) GetPlaybackHistory(ctx context.Context, req *PlaybackHistoryRequest) ([]PlaybackHistory, int64, error) {
    // 直接查询 Emby 数据库的 ActivityLog 表
    query := `
        SELECT 
            al.Id,
            al.UserId,
            u.Name as Username,
            al.Name as ItemName,
            al.Type as ItemType,
            al.DateCreated as PlayedAt,
            al.DeviceName,
            al.ClientName,
            CAST(json_extract(al.LogSeverity, '$.PlayDuration') AS INTEGER) as PlayDuration
        FROM ActivityLog al
        LEFT JOIN Users u ON al.UserId = u.Id
        WHERE al.Type IN ('VideoPlayback', 'AudioPlayback')
    `

    if req.UserID != nil {
        query += fmt.Sprintf(" AND al.UserId = '%s'", *req.UserID)
    }

    if req.Keyword != "" {
        query += fmt.Sprintf(" AND al.Name LIKE '%%%s%%'", req.Keyword)
    }

    if !req.StartDate.IsZero() {
        query += fmt.Sprintf(" AND al.DateCreated >= '%s'", req.StartDate.Format("2006-01-02"))
    }

    if !req.EndDate.IsZero() {
        query += fmt.Sprintf(" AND al.DateCreated <= '%s'", req.EndDate.Format("2006-01-02"))
    }

    query += " ORDER BY al.DateCreated DESC"
    query += fmt.Sprintf(" LIMIT %d OFFSET %d", req.PageSize, (req.Page-1)*req.PageSize)

    var history []PlaybackHistory
    if err := s.embyDB.Raw(query).Scan(&history).Error; err != nil {
        return nil, 0, err
    }

    // 格式化播放时长
    for i := range history {
        history[i].PlayDurationFormatted = formatDuration(history[i].PlayDuration)
    }

    // 获取总数
    var total int64
    countQuery := strings.Replace(query, "SELECT ... FROM", "SELECT COUNT(*) FROM", 1)
    s.embyDB.Raw(countQuery).Count(&total)

    return history, total, nil
}

func formatDuration(seconds int) string {
    hours := seconds / 3600
    minutes := (seconds % 3600) / 60
    if hours > 0 {
        return fmt.Sprintf("%dh %dm", hours, minutes)
    }
    return fmt.Sprintf("%dm", minutes)
}
```

---

## 前端页面

```vue
<template>
  <div class="playback-history">
    <el-row :gutter="20" class="toolbar">
      <el-col :span="8">
        <el-input v-model="keyword" placeholder="搜索媒体名称" @input="fetchHistory" />
      </el-col>
      <el-col :span="8">
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          @change="fetchHistory"
        />
      </el-col>
      <el-col :span="8">
        <el-select v-model="selectedUser" placeholder="选择用户" @change="fetchHistory">
          <el-option label="全部用户" :value="null" />
          <el-option v-for="user in users" :key="user.id" :label="user.username" :value="user.id" />
        </el-select>
      </el-col>
    </el-row>

    <el-table :data="history" style="margin-top: 20px">
      <el-table-column prop="username" label="用户" width="120" />
      <el-table-column prop="itemName" label="媒体名称" />
      <el-table-column prop="itemType" label="类型" width="100" />
      <el-table-column prop="playedAt" label="播放时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.playedAt) }}
        </template>
      </el-table-column>
      <el-table-column prop="deviceName" label="设备" width="150" />
      <el-table-column prop="clientName" label="客户端" width="120" />
      <el-table-column prop="playDurationFormatted" label="时长" width="100" />
    </el-table>

    <el-pagination
      v-model:current-page="currentPage"
      v-model:page-size="pageSize"
      :total="total"
      layout="total, sizes, prev, pager, next"
      @current-change="fetchHistory"
      @size-change="fetchHistory"
    />
  </div>
</template>
```

---

## 实施清单

- [ ] 实现服务层逻辑（查询 Emby 数据库）
- [ ] 实现 API 端点
- [ ] 实现前端页面
- [ ] 编写测试

**预计工作量**：2 天
