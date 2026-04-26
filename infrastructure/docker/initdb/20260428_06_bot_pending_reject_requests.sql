CREATE TABLE IF NOT EXISTS bot_pending_reject_requests (
  id              varchar(25)  PRIMARY KEY,
  "chatId"        bigint       NOT NULL,
  "adminUserId"   varchar(25)  NOT NULL,
  "subscriptionId" varchar(25) NOT NULL,
  "createdAt"     timestamptz  NOT NULL DEFAULT now(),
  "expiresAt"     timestamptz  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_bot_pending_reject_requests_chat
  ON bot_pending_reject_requests ("chatId", "expiresAt");
CREATE INDEX IF NOT EXISTS idx_bot_pending_reject_requests_expires
  ON bot_pending_reject_requests ("expiresAt");
