-- Ember migration: 新增设备管理相关表
-- Date: 2026-03-05
--
-- Purpose:
-- 1) 新增 client_blacklists（客户端黑名单）
-- 2) 新增 device_actions（设备操作日志）
--
-- Notes:
-- - 脚本幂等，可重复执行。
-- - 字段名采用 camelCase，需双引号包裹。

BEGIN;

CREATE TABLE IF NOT EXISTS client_blacklists (
  id                     varchar(25)  PRIMARY KEY,
  "clientName"           varchar(100) NOT NULL,
  "normalizedClientName" varchar(100) NOT NULL,
  reason                 varchar(255) NOT NULL DEFAULT '',
  "createdAt"            timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_client_blacklists_normalized_client_name
  ON client_blacklists ("normalizedClientName");

CREATE TABLE IF NOT EXISTS device_actions (
  id           varchar(25) PRIMARY KEY,
  "deviceId"   varchar(100) NOT NULL DEFAULT '',
  "userId"     varchar(25)  NOT NULL DEFAULT '',
  "clientName" varchar(100) NOT NULL DEFAULT '',
  action       varchar(50)  NOT NULL,
  note         varchar(255) NOT NULL DEFAULT '',
  "createdAt"  timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_device_actions_device_id
  ON device_actions ("deviceId");

CREATE INDEX IF NOT EXISTS idx_device_actions_user_id
  ON device_actions ("userId");

COMMIT;
