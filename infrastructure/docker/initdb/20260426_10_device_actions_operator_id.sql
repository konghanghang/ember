ALTER TABLE device_actions
  ADD COLUMN IF NOT EXISTS "operator_id" varchar(25);
CREATE INDEX IF NOT EXISTS idx_device_actions_operator
  ON device_actions ("operator_id")
  WHERE "operator_id" IS NOT NULL;
