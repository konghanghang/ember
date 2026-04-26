ALTER TABLE device_actions
  ADD COLUMN IF NOT EXISTS "operatorId" varchar(25);
CREATE INDEX IF NOT EXISTS idx_device_actions_operator
  ON device_actions ("operatorId")
  WHERE "operatorId" IS NOT NULL;
