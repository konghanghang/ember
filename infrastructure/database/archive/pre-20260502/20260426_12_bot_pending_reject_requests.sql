CREATE TABLE IF NOT EXISTS bot_pending_reject_requests (
  id              varchar(25)  PRIMARY KEY,
  "chat_id"        bigint       NOT NULL,
  "admin_user_id"   varchar(25)  NOT NULL,
  "subscription_id" varchar(25) NOT NULL,
  "created_at"     timestamptz  NOT NULL DEFAULT now(),
  "expires_at"     timestamptz  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_bot_pending_reject_requests_chat
  ON bot_pending_reject_requests ("chat_id", "expires_at");
CREATE INDEX IF NOT EXISTS idx_bot_pending_reject_requests_expires
  ON bot_pending_reject_requests ("expires_at");
