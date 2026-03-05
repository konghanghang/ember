# P1-3: 用户画像（User Profile Analytics）

## 功能描述

24 小时观影时间分布热力图、设备分布统计、趣味勋章系统（修仙党、周末狂欢、Emby 肝帝）。

**核心价值**：
- 了解用户观影习惯
- 趣味化用户体验
- 社区氛围营造

**优先级**：P1（高价值功能）⭐⭐⭐

---

## API 端点设计

```
GET /api/users/:userId/profile
Response:
{
  "data": {
    "hourlyDistribution": [
      { "hour": 0, "count": 5 },
      { "hour": 1, "count": 2 },
      ...
      { "hour": 23, "count": 10 }
    ],
    "deviceDistribution": [
      { "deviceName": "iPhone", "count": 50 },
      { "deviceName": "Web Browser", "count": 30 }
    ],
    "badges": [
      {
        "id": "night_owl",
        "name": "修仙党",
        "description": "凌晨 2-6 点观影超过 10 次",
        "icon": "🦉",
        "earnedAt": "2026-03-01T00:00:00Z"
      },
      {
        "id": "weekend_warrior",
        "name": "周末狂欢",
        "description": "周末观影时长超过 20 小时",
        "icon": "🎉",
        "earnedAt": "2026-02-28T00:00:00Z"
      }
    ],
    "totalPlayCount": 500,
    "totalPlayDuration": 360000,
    "favoriteGenre": "科幻"
  }
}
```

---

## 核心逻辑

```go
// GetUserProfile 获取用户画像
func (s *UserProfileService) GetUserProfile(ctx context.Context, userID uint) (*UserProfile, error) {
    var user models.User
    if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
        return nil, err
    }

    profile := &UserProfile{}

    // 1. 24 小时观影分布
    profile.HourlyDistribution = s.getHourlyDistribution(ctx, user.EmbyUserID)

    // 2. 设备分布
    profile.DeviceDistribution = s.getDeviceDistribution(ctx, user.EmbyUserID)

    // 3. 勋章系统
    profile.Badges = s.calculateBadges(ctx, user.EmbyUserID)

    // 4. 统计数据
    profile.TotalPlayCount, profile.TotalPlayDuration = s.getPlayStats(ctx, user.EmbyUserID)

    // 5. 最喜欢的类型
    profile.FavoriteGenre = s.getFavoriteGenre(ctx, user.EmbyUserID)

    return profile, nil
}

// calculateBadges 计算勋章
func (s *UserProfileService) calculateBadges(ctx context.Context, embyUserID string) []Badge {
    badges := []Badge{}

    // 修仙党：凌晨 2-6 点观影超过 10 次
    nightCount := s.countPlaybackInHours(ctx, embyUserID, 2, 6)
    if nightCount >= 10 {
        badges = append(badges, Badge{
            ID:          "night_owl",
            Name:        "修仙党",
            Description: "凌晨 2-6 点观影超过 10 次",
            Icon:        "🦉",
        })
    }

    // 周末狂欢：周末观影时长超过 20 小时
    weekendDuration := s.getWeekendPlayDuration(ctx, embyUserID)
    if weekendDuration >= 72000 { // 20 小时 = 72000 秒
        badges = append(badges, Badge{
            ID:          "weekend_warrior",
            Name:        "周末狂欢",
            Description: "周末观影时长超过 20 小时",
            Icon:        "🎉",
        })
    }

    // Emby 肝帝：单日观影超过 12 小时
    maxDailyDuration := s.getMaxDailyPlayDuration(ctx, embyUserID)
    if maxDailyDuration >= 43200 { // 12 小时 = 43200 秒
        badges = append(badges, Badge{
            ID:          "hardcore_viewer",
            Name:        "Emby 肝帝",
            Description: "单日观影超过 12 小时",
            Icon:        "💪",
        })
    }

    return badges
}
```

---

## 前端页面

```vue
<template>
  <div class="user-profile">
    <el-card title="观影时间分布">
      <div ref="hourlyChart" style="height: 300px"></div>
    </el-card>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="12">
        <el-card title="设备分布">
          <div ref="deviceChart" style="height: 300px"></div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card title="勋章墙">
          <div class="badges">
            <div v-for="badge in profile.badges" :key="badge.id" class="badge-item">
              <div class="badge-icon">{{ badge.icon }}</div>
              <div class="badge-name">{{ badge.name }}</div>
              <div class="badge-desc">{{ badge.description }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card title="统计数据" style="margin-top: 20px">
      <el-descriptions :column="3">
        <el-descriptions-item label="总播放次数">{{ profile.totalPlayCount }}</el-descriptions-item>
        <el-descriptions-item label="总观影时长">{{ formatDuration(profile.totalPlayDuration) }}</el-descriptions-item>
        <el-descriptions-item label="最喜欢的类型">{{ profile.favoriteGenre }}</el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<style scoped>
.badges {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
}
.badge-item {
  text-align: center;
  padding: 15px;
  border: 1px solid #eee;
  border-radius: 8px;
  width: 150px;
}
.badge-icon {
  font-size: 48px;
  margin-bottom: 10px;
}
.badge-name {
  font-weight: bold;
  margin-bottom: 5px;
}
.badge-desc {
  font-size: 12px;
  color: #909399;
}
</style>
```

---

## 实施清单

- [ ] 实现服务层逻辑
- [ ] 实现勋章计算引擎
- [ ] 实现 API 端点
- [ ] 实现前端页面
- [ ] 集成 ECharts 热力图
- [ ] 编写测试

**预计工作量**：3-4 天
