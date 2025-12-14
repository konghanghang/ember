-- Phase 2: 订阅系统数据库迁移
-- 创建时间: 2025-12-14
-- 说明: 新增订阅表、枚举类型和 posterPath 字段

-- CreateEnum
CREATE TYPE "SubscriptionStatus" AS ENUM ('PENDING', 'APPROVED', 'REJECTED');
CREATE TYPE "MediaType" AS ENUM ('MOVIE', 'TV');

-- CreateTable
CREATE TABLE "subscriptions" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "type" "MediaType" NOT NULL,
    "name" TEXT NOT NULL,
    "tmdbId" TEXT NOT NULL,
    "posterPath" TEXT,
    "status" "SubscriptionStatus" NOT NULL DEFAULT 'PENDING',
    "note" TEXT,
    "mpError" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "subscriptions_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "subscriptions_userId_idx" ON "subscriptions"("userId");
CREATE INDEX "subscriptions_status_idx" ON "subscriptions"("status");
CREATE INDEX "subscriptions_createdAt_idx" ON "subscriptions"("createdAt");

-- AddForeignKey
ALTER TABLE "subscriptions" ADD CONSTRAINT "subscriptions_userId_fkey"
    FOREIGN KEY ("userId") REFERENCES "users"("id")
    ON DELETE CASCADE ON UPDATE CASCADE;

-- 字段说明:
-- 1. SubscriptionStatus: 订阅状态（待审核/已批准/已拒绝）
-- 2. MediaType: 媒体类型（电影/电视剧）
-- 3. posterPath: TMDB 封面图片路径（可选，如 /path/to/poster.jpg）
-- 4. mpError: MoviePilot 同步错误信息
--    - status='APPROVED' + mpError=null: 已同步成功
--    - status='APPROVED' + mpError='xxx': 同步失败，允许管理员重试
-- 5. onDelete CASCADE: 用户删除时级联删除其订阅记录
