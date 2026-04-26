-- media_gap_scans：缺集扫描的持久化执行记录。
--
-- 业务背景：
--   handlers/media_gap_async.go 使用进程内 sync.RWMutex 控制扫描互斥，多副本部署时
--   每个副本各自跑全库扫描，把上游 Emby / TMDB 流量放大 N 倍。本批引入 PostgreSQL
--   advisory lock + 本表，提供跨副本互斥与扫描审计记录。
--
-- 字段说明：
--   id            扫描批次 ID（cuid）
--   status        running / success / failed
--   nodeId        承担本次扫描的节点标识（hostname + pid，便于运维回溯）
--   startedAt     扫描开始时间
--   finishedAt    扫描结束时间（success/failed 时填充）
--   errorMessage  失败时的脱敏错误（最多 500 字符）
--
-- 配套 cron `media-gap-scans-cleanup` (@weekly) 清理 7 天前的 success/failed 记录，
-- 避免长期堆积；running 记录不被清理，需运维手工排查（一般来源于进程 crash 未释放）。
--
-- 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS media_gap_scans (
    id             varchar(25)  PRIMARY KEY,
    status         varchar(20)  NOT NULL,
    "nodeId"       varchar(64)  NOT NULL,
    "startedAt"    timestamptz  NOT NULL DEFAULT now(),
    "finishedAt"   timestamptz,
    "errorMessage" varchar(500)
);

CREATE INDEX IF NOT EXISTS idx_media_gap_scans_started
    ON media_gap_scans ("startedAt");
