-- Drop obsolete redemption code template-user runtime field.
-- Existing empty registration_plan_group values are backfilled to the single default plan group
-- before the column is tightened to NOT NULL.

DO $$
DECLARE
  default_group_key varchar(50);
  default_group_count integer;
BEGIN
  SELECT COUNT(*), MAX(key)
    INTO default_group_count, default_group_key
    FROM plan_groups
    WHERE is_default = true;

  IF default_group_count <> 1 THEN
    RAISE EXCEPTION 'expected exactly one default plan group before backfilling redemption_codes.registration_plan_group, got %', default_group_count;
  END IF;

  UPDATE redemption_codes
    SET registration_plan_group = default_group_key
    WHERE registration_plan_group IS NULL OR btrim(registration_plan_group) = '';
END $$;

ALTER TABLE redemption_codes
  ALTER COLUMN registration_plan_group SET NOT NULL;

DROP INDEX IF EXISTS idx_redemption_codes_template_user_id;

ALTER TABLE redemption_codes
  DROP COLUMN IF EXISTS template_user_id;
