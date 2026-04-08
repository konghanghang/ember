-- Ember migration: add managed plan groups with default fallback
-- Date: 2026-04-08
--
-- Purpose:
-- 1) Add plan_groups table so plan grouping becomes a managed entity
-- 2) Keep plans bound to an explicit group key
-- 3) Allow users.planGroup to be NULL, which means "follow default group"
-- 4) Initialize exactly one default group for the brand-new feature rollout
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing plans fall back to the default group when they have no group yet.
-- - Existing users keep NULL so they follow the default group.
-- - Database layer intentionally does not add foreign keys; group existence is enforced in services.

BEGIN;

CREATE TABLE IF NOT EXISTS plan_groups (
  key varchar(50) PRIMARY KEY,
  name varchar(100) NOT NULL,
  description varchar(500) NOT NULL DEFAULT '',
  "isDefault" boolean NOT NULL DEFAULT false,
  "sortOrder" integer NOT NULL DEFAULT 0,
  "createdAt" timestamptz NOT NULL DEFAULT NOW(),
  "updatedAt" timestamptz NOT NULL DEFAULT NOW()
);

INSERT INTO plan_groups (key, name, description, "isDefault", "sortOrder")
VALUES
  ('DEFAULT', '默认分组', '系统默认套餐分组', true, 10)
ON CONFLICT (key) DO NOTHING;

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS "planGroup" varchar(50);

ALTER TABLE plans
  ADD COLUMN IF NOT EXISTS "planGroup" varchar(50);

UPDATE plans
SET "planGroup" = 'DEFAULT'
WHERE "planGroup" IS NULL OR btrim("planGroup") = '';

UPDATE users
SET "planGroup" = NULL
WHERE "planGroup" IS NOT NULL AND btrim("planGroup") = '';

ALTER TABLE plans
  ALTER COLUMN "planGroup" TYPE varchar(50);

ALTER TABLE users
  ALTER COLUMN "planGroup" TYPE varchar(50);

ALTER TABLE plans
  ALTER COLUMN "planGroup" SET NOT NULL;

ALTER TABLE plans
  ALTER COLUMN "planGroup" SET DEFAULT 'DEFAULT';

ALTER TABLE users
  ALTER COLUMN "planGroup" DROP NOT NULL;

ALTER TABLE users
  ALTER COLUMN "planGroup" DROP DEFAULT;

CREATE INDEX IF NOT EXISTS idx_plan_groups_is_default
  ON plan_groups ("isDefault");

CREATE UNIQUE INDEX IF NOT EXISTS uq_plan_groups_default_true
  ON plan_groups ("isDefault")
  WHERE "isDefault" = true;

CREATE INDEX IF NOT EXISTS idx_plan_groups_sort_order
  ON plan_groups ("sortOrder");

CREATE INDEX IF NOT EXISTS idx_users_plan_group
  ON users ("planGroup");

CREATE INDEX IF NOT EXISTS idx_plans_plan_group
  ON plans ("planGroup");

COMMIT;
