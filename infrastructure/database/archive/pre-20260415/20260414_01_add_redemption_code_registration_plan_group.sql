-- Ember migration: add registration-only plan group binding to redemption codes
-- Date: 2026-04-14
--
-- Purpose:
-- 1) Add `registrationPlanGroup` column to redemption_codes
-- 2) Support admin-side filtering and registration-time plan group binding
--
-- Notes:
-- - Script is idempotent and safe to run multiple times.
-- - Existing redemption codes keep NULL and preserve historical behavior.
-- - Database layer intentionally does not add foreign keys; group existence is enforced in services.

BEGIN;

ALTER TABLE redemption_codes
  ADD COLUMN IF NOT EXISTS "registrationPlanGroup" varchar(50);

ALTER TABLE redemption_codes
  ALTER COLUMN "registrationPlanGroup" TYPE varchar(50);

CREATE INDEX IF NOT EXISTS idx_redemption_codes_registration_plan_group
  ON redemption_codes ("registrationPlanGroup");

COMMIT;
