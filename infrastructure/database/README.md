# 数据库迁移（PostgreSQL）

本目录存放 Ember 项目的数据库迁移 SQL。

## 使用方式

1. **生产/已有数据库升级**

```bash
psql "$DATABASE_URL" -f infrastructure/database/20260215_01_create_playback_rankings.sql

# 追剧日历功能迁移（2026-03-05）
psql "$DATABASE_URL" -f infrastructure/database/20260305_03_add_tv_calendar_tables.sql

# 设置中心配置表扩展（2026-03-12）
psql "$DATABASE_URL" -f infrastructure/database/20260312_01_expand_settings_for_config_center.sql
```

2. **Docker 首次初始化（仅首次）**

`infrastructure/docker/docker-compose.yml` 会把本目录挂载到 Postgres 的 `/docker-entrypoint-initdb.d`。

- 只在数据库数据卷为空时执行一次
- 如果数据库已存在，需要用上面的 `psql` 手动执行迁移
