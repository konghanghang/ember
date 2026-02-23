-- Ember migration: add stripe plans and payments tables
-- Date: 2026-02-22
--
-- Purpose:
-- 1) Create plans table for admin-managed pricing plans
-- 2) Create payments table for Stripe checkout records
--
-- Notes:
-- - CamelCase columns are quoted to match current GORM model tags.
-- - Script is idempotent and safe to run multiple times.

BEGIN;

CREATE TABLE IF NOT EXISTS plans (
  id            varchar(25)  PRIMARY KEY,
  name          varchar(100) NOT NULL,
  description   varchar(500) NOT NULL DEFAULT '',
  days          integer      NOT NULL,
  price         bigint       NOT NULL,
  currency      varchar(3)   NOT NULL DEFAULT 'usd',
  "isActive"    boolean      NOT NULL DEFAULT true,
  "sortOrder"   integer      NOT NULL DEFAULT 0,
  "createdAt"   timestamptz  NOT NULL DEFAULT now(),
  "updatedAt"   timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_plans_active_sort
  ON plans ("isActive", "sortOrder");

CREATE TABLE IF NOT EXISTS payments (
  id                       varchar(25)  PRIMARY KEY,
  "userId"                 varchar(25)  NOT NULL,
  "planId"                 varchar(25)  NOT NULL,
  "stripeSessionId"        varchar(255) NOT NULL,
  "stripePaymentIntentId"  varchar(255) NOT NULL DEFAULT '',
  amount                   bigint       NOT NULL,
  currency                 varchar(3)   NOT NULL DEFAULT 'usd',
  days                     integer      NOT NULL,
  status                   varchar(20)  NOT NULL DEFAULT 'pending',
  "createdAt"              timestamptz  NOT NULL DEFAULT now(),
  "updatedAt"              timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_stripe_session
  ON payments ("stripeSessionId");

CREATE INDEX IF NOT EXISTS idx_payments_user_id
  ON payments ("userId");

CREATE INDEX IF NOT EXISTS idx_payments_plan_id
  ON payments ("planId");

COMMIT;
