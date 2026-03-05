# P1-1: 媒体库质量盘点（Media Quality Insight）

## 功能描述

深度扫描媒体库（电影/剧集），统计分辨率、编码格式、HDR 类型，自动列出低画质资源供洗版参考。24 小时缓存机制。

**核心价值**：
- 快速识别低画质资源
- 优化媒体库质量
- 数据驱动的洗版决策

**优先级**：P1（高价值功能）⭐⭐⭐⭐

---

## 数据模型设计

### Media Quality Cache 表（media_quality_cache）

```go
type MediaQualityCache struct {
    ID         uint      `gorm:"primaryKey;column:id" json:"id"`
    LibraryID  string    `gorm:"column:library_id;size:50;uniqueIndex" json:"libraryId"`
    Statistics string    `gorm:"column:statistics;type:text" json:"statistics"` // JSON
    ExpiresAt  time.Time `gorm:"column:expires_at;index" json:"expiresAt"`
    CreatedAt  time.Time `gorm:"column:created_at" json:"createdAt"`
}
```

---

## API 端点设计

```
GET /api/media-quality/scan/:libraryId
Query Parameters:
  - force: bool (强制刷新缓存)

Response:
{
  "data": {
    "resolutionDistribution": [
      { "resolution": "4K", "count": 150 },
      { "resolution": "1080p", "count": 500 },
      { "resolution": "720p", "count": 50 }
    ],
    "codecDistribution": [
      { "codec": "HEVC", "count": 400 },
      { "codec": "H.264", "count": 300 }
    ],
    "hdrDistribution": [
      { "type": "HDR10", "count": 100 },
      { "type": "Dolby Vision", "count": 50 },
      { "type": "SDR", "count": 550 }
    ],
    "lowQualityItems": [
      {
        "id": "item123",
        "name": "Movie Name",
        "resolution": "720p",
        "codec": "H.264",
        "bitrate": 2000
      }
    ]
  }
}
```

---

## 核心逻辑

```go
// ScanLibraryQuality 扫描媒体库质量
func (s *MediaQualityService) ScanLibraryQuality(ctx context.Context, libraryID string, force bool) (*QualityReport, error) {
    // 1. 检查缓存
    if !force {
        var cache models.MediaQualityCache
        if err := s.db.WithContext(ctx).
            Where("library_id = ? AND expires_at > ?", libraryID, time.Now()).
            First(&cache).Error; err == nil {
            var report QualityReport
            json.Unmarshal([]byte(cache.Statistics), &report)
            return &report, nil
        }
    }

    // 2. 从 Emby 获取媒体库所有条目
    items, err := s.embyService.GetLibraryItems(ctx, libraryID)
    if err != nil {
        return nil, err
    }

    // 3. 分析每个条目的 MediaInfo
    report := &QualityReport{}
    for _, item := range items {
        mediaInfo, err := s.embyService.GetMediaInfo(ctx, item.ID)
        if err != nil {
            continue
        }

        // 统计分辨率
        resolution := s.getResolution(mediaInfo)
        report.ResolutionDistribution[resolution]++

        // 统计编码格式
        codec := mediaInfo.MediaStreams[0].Codec
        report.CodecDistribution[codec]++

        // 统计 HDR
        hdrType := s.getHDRType(mediaInfo)
        report.HDRDistribution[hdrType]++

        // 识别低画质资源
        if s.isLowQuality(mediaInfo) {
            report.LowQualityItems = append(report.LowQualityItems, item)
        }
    }

    // 4. 保存缓存
    cacheData, _ := json.Marshal(report)
    s.db.WithContext(ctx).Create(&models.MediaQualityCache{
        LibraryID:  libraryID,
        Statistics: string(cacheData),
        ExpiresAt:  time.Now().Add(24 * time.Hour),
    })

    return report, nil
}
```

---

## 前端页面

```vue
<template>
  <div class="media-quality">
    <el-select v-model="selectedLibrary" @change="scanLibrary">
      <el-option v-for="lib in libraries" :key="lib.id" :label="lib.name" :value="lib.id" />
    </el-select>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="8">
        <el-card title="分辨率分布">
          <div ref="resolutionChart" style="height: 300px"></div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card title="编码格式分布">
          <div ref="codecChart" style="height: 300px"></div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card title="HDR 分布">
          <div ref="hdrChart" style="height: 300px"></div>
        </el-card>
      </el-col>
    </el-row>

    <el-card title="低画质资源" style="margin-top: 20px">
      <el-table :data="lowQualityItems">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="resolution" label="分辨率" width="100" />
        <el-table-column prop="codec" label="编码" width="100" />
        <el-table-column prop="bitrate" label="码率" width="100" />
      </el-table>
    </el-card>
  </div>
</template>
```

---

## 实施清单

- [ ] 创建数据模型
- [ ] 实现扫描服务
- [ ] 实现 API 端点
- [ ] 实现前端页面
- [ ] 集成 ECharts 图表
- [ ] 编写测试

**预计工作量**：3-4 天
