/**
 * 邀请码相关的工具函数
 *
 * 职责：
 * 1. 纯函数：邀请码验证逻辑
 * 2. 工具函数：邀请码生成、日志记录
 */

import type { Invite } from '@prisma/client'

// ==================== 纯函数：邀请码验证 ====================

/**
 * 判断邀请码是否已过期
 * @param invite 邀请码对象
 * @returns 是否已过期
 */
export function isInviteExpired(invite: Pick<Invite, 'expiresAt'>): boolean {
  if (!invite.expiresAt) return false
  return invite.expiresAt < new Date()
}

/**
 * 判断邀请码是否已用尽
 * @param invite 邀请码对象
 * @returns 是否已用尽
 */
export function isInviteUsedUp(invite: Pick<Invite, 'usedCount' | 'maxUses'>): boolean {
  return invite.usedCount >= invite.maxUses
}

/**
 * 判断邀请码是否可用
 *
 * 条件：
 * 1. 使用次数未达上限
 * 2. 未过期（如果设置了过期时间）
 *
 * @param invite 邀请码对象
 * @returns { valid: boolean, reason?: string }
 */
export function canInviteBeUsed(
  invite: Pick<Invite, 'usedCount' | 'maxUses' | 'expiresAt'>
): { valid: boolean; reason?: string } {
  // 1. 检查使用次数
  if (isInviteUsedUp(invite)) {
    return { valid: false, reason: '邀请码已达使用上限' }
  }

  // 2. 检查是否过期
  if (isInviteExpired(invite)) {
    return { valid: false, reason: '邀请码已过期' }
  }

  return { valid: true }
}

/**
 * 判断邀请码是否可以被删除
 * @param invite 邀请码对象
 * @returns 是否可以删除
 */
export function canInviteBeDeleted(invite: Pick<Invite, 'usedCount'>): boolean {
  return invite.usedCount === 0
}
