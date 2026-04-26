-- 幂等：重复执行安全
-- 1. 预清理重复 batch（保留每组最新一条）
WITH duplicates AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY period, period_start, period_end
           ORDER BY "createdAt" DESC
         ) AS rn
  FROM playback_rankings
)
DELETE FROM playback_rankings
WHERE id IN (SELECT id FROM duplicates WHERE rn > 1);

-- 2. 扩位 batch_id
ALTER TABLE playback_rankings
  ALTER COLUMN batch_id TYPE varchar(32);

-- 3. 添加幂等唯一索引
CREATE UNIQUE INDEX IF NOT EXISTS uq_playback_rankings_period
  ON playback_rankings (period, period_start, period_end);
