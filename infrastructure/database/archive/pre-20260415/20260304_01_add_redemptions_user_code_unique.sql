-- Ember migration: 为 redemptions 增加一人一码唯一约束
-- Date: 2026-03-04
--
-- Purpose:
-- 1) 清理历史重复兑换记录（按 userId+code，仅保留最早一条）
-- 2) 增加唯一索引，强制“一人一码一次”
-- 3) 校准 redemption_codes.usedCount 与 redemptions 实际记录数一致
--
-- Notes:
-- - 脚本可重复执行（幂等）。
-- - 若线上表很大且对写入锁敏感，可改为 CREATE UNIQUE INDEX CONCURRENTLY 并拆分事务执行。

BEGIN;

-- 1) 去重：同一 userId + code 仅保留最早一条
WITH ranked AS (
  SELECT
    ctid,
    ROW_NUMBER() OVER (
      PARTITION BY "userId", code
      ORDER BY "createdAt" ASC, id ASC
    ) AS rn
  FROM redemptions
)
DELETE FROM redemptions r
USING ranked x
WHERE r.ctid = x.ctid
  AND x.rn > 1;

-- 2) 唯一索引：一人一码一次
CREATE UNIQUE INDEX IF NOT EXISTS uq_redemptions_user_code
  ON redemptions ("userId", code);

-- 3) usedCount 校准（有兑换记录的码）
UPDATE redemption_codes rc
SET "usedCount" = s.cnt
FROM (
  SELECT code, COUNT(*)::integer AS cnt
  FROM redemptions
  GROUP BY code
) s
WHERE rc.code = s.code;

-- 4) usedCount 校准（无兑换记录的码）
UPDATE redemption_codes rc
SET "usedCount" = 0
WHERE rc."usedCount" <> 0
  AND NOT EXISTS (
    SELECT 1
    FROM redemptions r
    WHERE r.code = rc.code
  );

COMMIT;
