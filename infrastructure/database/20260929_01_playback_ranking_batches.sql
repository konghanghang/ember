-- 排行榜批次与明细：周期唯一性归批次，明细按批次/类别/排名唯一。
-- 涉及 playback_ranking_batches、playback_rankings；旧记录原样保留并回填批次。
-- 历史 total_duration 未曾完整持久化，保留 NULL，不从残存明细推测。
-- 可重复执行；由启动期迁移器统一包裹事务，不触发外部查询或历史推送。

CREATE TABLE IF NOT EXISTS playback_ranking_batches (
    id VARCHAR(32) PRIMARY KEY,
    period VARCHAR(10) NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    snapshot_at TIMESTAMPTZ NOT NULL,
    total_duration BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_playback_ranking_batches_period
    ON playback_ranking_batches (period, period_start, period_end);

-- 原周期唯一索引保证现行历史库每期至多一行；按周期聚合同时兼容完整的旧批次。
-- 使用原行 ID 构造稳定标识，避免依赖迁移会话时区或不可重复的随机值。
WITH legacy_periods AS (
    SELECT period, period_start, period_end, MIN(id) AS anchor_id
    FROM playback_rankings
    WHERE NULLIF(BTRIM(batch_id), '') IS NULL
    GROUP BY period, period_start, period_end
)
UPDATE playback_rankings AS ranking
SET batch_id = 'legacy_' || legacy_periods.anchor_id
FROM legacy_periods
WHERE NULLIF(BTRIM(ranking.batch_id), '') IS NULL
  AND ranking.period = legacy_periods.period
  AND ranking.period_start = legacy_periods.period_start
  AND ranking.period_end = legacy_periods.period_end;

INSERT INTO playback_ranking_batches (id, period, period_start, period_end, snapshot_at, total_duration, created_at)
SELECT batch_id, period, period_start, period_end, MAX(snapshot_at), NULL, COALESCE(MIN(created_at), MAX(snapshot_at))
FROM playback_rankings
GROUP BY batch_id, period, period_start, period_end
ON CONFLICT (id) DO NOTHING;

DROP INDEX IF EXISTS uq_playback_rankings_period;

CREATE UNIQUE INDEX IF NOT EXISTS uq_playback_rankings_batch_rank
    ON playback_rankings (batch_id, category, rank);
DROP INDEX IF EXISTS idx_ranking_batch;

ALTER TABLE playback_rankings ALTER COLUMN batch_id DROP DEFAULT;
ALTER TABLE playback_rankings ALTER COLUMN batch_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_playback_rankings_batch'
          AND conrelid = 'playback_rankings'::regclass
    ) THEN
        ALTER TABLE playback_rankings
            ADD CONSTRAINT fk_playback_rankings_batch
            FOREIGN KEY (batch_id) REFERENCES playback_ranking_batches(id) ON DELETE CASCADE;
    END IF;
END;
$$;
