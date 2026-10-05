-- 兑换码及兑换历史增加显式权益有效期类型；不改变兑换码本身的 expires_at。
-- 旧数据通过 DEFAULT duration 回填，天数、使用次数和已发放权益保持不变。
-- 可重复执行；不重发权益，不从当前兑换码反推历史记录。
ALTER TABLE redemption_codes ADD COLUMN IF NOT EXISTS validity_type varchar(20) NOT NULL DEFAULT 'duration';
ALTER TABLE redemptions ADD COLUMN IF NOT EXISTS validity_type varchar(20) NOT NULL DEFAULT 'duration';

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_redemption_codes_validity' AND conrelid = 'redemption_codes'::regclass) THEN
        ALTER TABLE redemption_codes ADD CONSTRAINT ck_redemption_codes_validity CHECK (
            (validity_type = 'duration' AND default_days > 0) OR
            (validity_type = 'permanent' AND default_days = 0)
        );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_redemptions_validity' AND conrelid = 'redemptions'::regclass) THEN
        ALTER TABLE redemptions ADD CONSTRAINT ck_redemptions_validity CHECK (
            validity_type = 'duration' OR (validity_type = 'permanent' AND days = 0)
        );
    END IF;
END $$;
