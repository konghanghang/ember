/**
 * 用户管理相关的工具函数
 *
 * 职责：
 * 1. 纯函数：时间计算、状态判断等
 * 2. 工具函数：统一的 Emby 同步、日志记录
 * 3. 错误处理：统一的错误消息格式
 */

import { add } from 'date-fns'
import type { User } from '@prisma/client'

// ==================== 纯函数：时间计算 ====================

/**
 * 计算用户新的到期时间
 * @param baseDate 基准日期（当前到期时间）
 * @param days 延长天数
 * @returns 新的到期时间
 */
export function calculateUserExpiry(baseDate: Date, days: number): Date {
  return add(baseDate, { days })
}

/**
 * 计算用户到期剩余天数
 * @param expiresAt 到期时间
 * @returns 剩余天数（负数表示已过期）
 */
export function getDaysUntilExpiry(expiresAt: Date | null): number {
  if (!expiresAt) return -1
  const now = new Date()
  const diff = expiresAt.getTime() - now.getTime()
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

/**
 * 判断用户是否已过期
 * @param user 用户对象
 * @returns 是否已过期
 */
export function isUserExpired(user: Pick<User, 'expiresAt'>): boolean {
  if (!user.expiresAt) return false
  return user.expiresAt < new Date()
}

// ==================== 纯函数：状态判断 ====================

/**
 * 判断是否需要重新启用用户
 *
 * 条件：用户当前被禁用 && 新到期时间在未来
 *
 * @param user 用户对象
 * @param newExpiry 新的到期时间
 * @returns 是否需要重新启用
 */
export function shouldReactivateUser(
  user: Pick<User, 'isActive'>,
  newExpiry: Date
): boolean {
  return !user.isActive && newExpiry > new Date()
}

/**
 * 获取用户切换后的目标状态
 * @param user 用户对象
 * @returns { isActive: 目标激活状态, isDisabled: 对应的 Emby 禁用状态 }
 */
export function getUserTargetStatus(user: Pick<User, 'isActive'>): {
  isActive: boolean
  isDisabled: boolean
} {
  const targetActive = !user.isActive
  return {
    isActive: targetActive,
    isDisabled: !targetActive, // Emby 的 IsDisabled 和 isActive 相反
  }
}

// ==================== 工具函数：日志记录 ====================

/**
 * 创建用户操作日志（统一格式）
 *
 * @param tx Prisma 事务对象
 * @param action 操作类型
 * @param targetId 目标用户 ID
 * @param details 详细信息
 */
export async function createUserLog(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  tx: any,
  action: string,
  targetId: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  details: Record<string, any>
) {
  await tx.log.create({
    data: {
      action,
      targetId,
      details,
    },
  })
}

// ==================== 工具函数：错误消息 ====================

/**
 * 格式化 Emby API 错误消息
 * @param _error 错误对象
 * @returns 用户友好的错误消息
 */
export function formatEmbyError(_error: unknown): string {
  return 'Emby 操作失败，请稍后重试'
}

/**
 * 格式化数据库错误消息
 * @param _error 错误对象
 * @returns 用户友好的错误消息
 */
export function formatDatabaseError(_error: unknown): string {
  return '数据库操作失败，请稍后重试'
}
