# 媒体重复版本治理与质量建议实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- Ember 当前已经有媒体质量盘点，但重点在分辨率、编码、HDR 分布和低质量条目，不覆盖重复版本治理。
- 服主在实际运维中更常遇到的问题是“同一部电影或同一集剧集留了多个版本，不知道该删谁”。
- 如果没有重复版本建议，媒体质量盘点只能看热闹，不能真正指导清理和洗版。

## 目标

本方案要实现：

1. 在现有媒体质量盘点之上增加“重复版本治理”能力。
2. 为重复版本组生成可解释的质量评分和建议保留项，帮助管理员决策。
3. 保持首版风险可控，不自动删除任何媒体文件。

## 非目标

本次明确不做：

- 不自动删除文件或直接操作 Emby 删除接口。
- 不尝试做跨媒体服务器、跨存储的全局去重。
- 不把“重复版本治理”扩展成通用转码/洗版编排系统。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/media/quality.go`
  - `services/api/internal/models/media_quality_cache.go`
  - `services/web/src/views/admin/MediaQualityView.vue`
- 当前行为：
  - 媒体质量服务会扫描库，生成分辨率、编码、HDR 分布及低质量条目。
  - 质量结果通过缓存表持久化，但没有重复版本专用模型和忽略机制。
  - 后台媒体质量页没有“重复版本组”“建议保留 / 建议清理”视图。
- 现有限制：
  - 服主需要借助外部工具或手工判断重复版本。
  - 现有质量报告无法告诉管理员“同组里哪一版更应该留”。
  - 误报场景没有白名单或忽略机制，无法做长期治理。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 在媒体质量页增加“重复版本治理”视图。
  - 展示重复版本组、评分、建议保留项、建议关注项。
  - 管理员可对某个重复组执行“忽略此组”，避免长期重复告警。
- 修改现有行为：
  - 媒体质量页从“低质量盘点”扩展为“低质量 + 重复版本”双视角治理页。
- 哪些现有行为必须保持不变：
  - 现有低质量报告、海报查看和分组详情能力保持可用。
  - 首版不提供“直接删除”按钮，避免误删。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 优先扩展现有 `MediaQualityView`，用 Tab 或分区承载，不新开平行页面。
  - 若展示评分说明和建议标签，仍需保持卡片与列表语义统一。

### 2. 数据与模型

- 新增 `media_duplicate_cache` 表：
  - `id`
  - `libraryId`
  - `statistics`：重复版本扫描结果 JSON
  - `expiresAt`
  - `createdAt`
  - `updatedAt`
- 新增 `media_duplicate_ignores` 表：
  - `id`
  - `groupKey`
  - `libraryId`
  - `reason`
  - `createdAt`
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 不修改现有 `media_quality_cache` 结构，避免混合两类缓存语义

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `GET /api/v1/admin/media-quality/libraries/:libraryId/duplicates`
    - 获取重复版本组列表
  - `GET /api/v1/admin/media-quality/libraries/:libraryId/duplicates/:groupKey`
    - 获取单组详情
  - `POST /api/v1/admin/media-quality/libraries/:libraryId/duplicates/scan`
    - 触发重复版本扫描
  - `POST /api/v1/admin/media-quality/libraries/:libraryId/duplicates/ignore`
    - 忽略某个重复组
  - `DELETE /api/v1/admin/media-quality/libraries/:libraryId/duplicates/ignore/:groupKey`
    - 取消忽略
- 请求参数与响应字段怎么变：
  - 重复版本结果返回明确的 `groupKey`、`score`、`recommendedKeep`
  - 评分构成只作为说明字段返回，不把前端耦合到后端内部实现细节
- 哪些调用方会受影响：
  - `MediaQualityView`
  - 媒体质量后台接口

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 管理员触发某个媒体库的重复版本扫描。
2. 系统拉取库内项目和媒体源详情，按电影或剧集季集身份分组。
3. 对组内每个版本按分辨率、码率、编码、HDR/DoVi、字幕等规则计算质量分。
4. 系统生成建议保留项和候选关注项，写入重复版本缓存。
5. 前端读取重复版本组列表，展示建议和评分摘要。
6. 管理员对误报组执行忽略，后续扫描默认不再展示该组。

### 5. 失败路径与边界条件

- Emby 返回媒体源不完整：该项可降级为“无法评分”，但不能导致整组扫描失败。
- ProviderId 缺失导致分组不稳定：先按可用键降级分组，并在结果中标记可信度不足。
- 合并版本与多媒体源场景：必须按媒体源而不是单个 Emby Item 粗暴判断，避免漏报。
- 忽略规则命中后再次扫描：结果仍写缓存，但前端默认不展示被忽略组。
- 兼容性约束：
  - 不能影响现有媒体质量低质量报告接口。
  - 首版不能提供自动删除，避免把“建议系统”变成“高风险执行器”。

## 影响范围

涉及的子系统：

- API：有
  - 重复版本扫描服务、缓存、忽略规则、接口
- Web：有
  - `MediaQualityView` 扩展
- Bot：无
- 配置/部署：无新增环境变量
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- 重复版本分组与评分逻辑
- 忽略规则命中逻辑
- 前端详情与推荐展示

### 手工验证

- 在存在重复版本的媒体库执行扫描，确认出现重复版本组
- 组内能看到建议保留项和评分理由摘要
- 忽略一组后重新扫描，确认默认不再显示
- 低质量盘点原有功能继续可用

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - 重复版本治理模型
  - 媒体质量与重复版本扫描的边界
- 如功能稳定，可补充后台运维参考文档
- 主体稳定后移入 `docs/archive/plan/media-subscription/`
