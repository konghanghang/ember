# 数据库备份 Runbook

## 概述

Ember 使用 PostgreSQL 作为数据库。本文档涵盖备份策略、命令样板和恢复步骤。

---

## 备份命令样板

### 全量备份（推荐生产使用）

```bash
pg_dump \
  --format=custom \
  --no-owner \
  --no-privileges \
  --compress=9 \
  "$DATABASE_URL" \
  -f "ember_backup_$(date +%Y%m%d_%H%M%S).dump"
```

### 仅 Schema（不含数据）

```bash
pg_dump \
  --schema-only \
  --no-owner \
  "$DATABASE_URL" \
  -f "ember_schema_$(date +%Y%m%d).sql"
```

### 仅数据（不含 DDL）

```bash
pg_dump \
  --data-only \
  --no-owner \
  "$DATABASE_URL" \
  -f "ember_data_$(date +%Y%m%d_%H%M%S).sql"
```

### Docker 容器内执行

```bash
docker exec ember-postgres pg_dump \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" \
  --format=custom \
  --compress=9 \
  -f /tmp/ember_backup.dump

docker cp ember-postgres:/tmp/ember_backup.dump ./backups/
```

---

## 备份周期建议

| 环境  | 全量备份  | 保留时间 |
|------|---------|---------|
| 生产  | 每日 02:00 | 30 天   |
| 预发  | 每周     | 7 天    |
| 本地  | 按需     | 手动清理  |

**建议方案**：使用 cron 每日自动备份，结合对象存储（S3/OSS）异地存储，本地只保留最近 7 天。

---

## 加密备份（GPG）

```bash
# 备份并加密
pg_dump --format=custom "$DATABASE_URL" | \
  gpg --symmetric --cipher-algo AES256 \
  -o "ember_backup_$(date +%Y%m%d_%H%M%S).dump.gpg"

# 解密
gpg -o ember_backup.dump -d ember_backup_YYYYMMDD_HHMMSS.dump.gpg
```

---

## 恢复步骤

### 前置检查

```bash
# 确认目标库为空或已创建
psql "$DATABASE_URL" -c "\dt"

# 确认备份文件完整
pg_restore --list ember_backup.dump | head -20
```

### 从 custom 格式恢复

```bash
pg_restore \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  -d "$DATABASE_URL" \
  ember_backup.dump
```

### 从 SQL 文件恢复

```bash
psql "$DATABASE_URL" -f ember_backup.sql
```

### 恢复后验证

```bash
# 检查关键表是否存在
psql "$DATABASE_URL" -c "\dt"

# 检查行数
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM users;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM subscriptions;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM payments;"
```

---

## 注意事项

- 生产恢复前必须先停止 API 服务，避免脏数据写入
- 恢复完成后按顺序运行 baseline 之后所有增量 migration（如果恢复的是旧备份）
- 不要直接把备份文件发送到不受信任的存储；加密后再传输
- 定期验证备份可恢复性，不要等到真正需要时才发现备份损坏
