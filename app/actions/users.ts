'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'
import { validateInvite } from './invites'
import {
  calculateUserExpiry,
  shouldReactivateUser,
  getUserTargetStatus,
  createUserLog,
  formatEmbyError,
  formatDatabaseError,
} from '@/lib/user-helpers'
import { notifyNewRegistration } from '@/lib/telegram'
import { generateRandomPassword } from '@/lib/auth-helpers'
import type { Invite } from '@prisma/client'

// ==================== 用户注册相关辅助函数 ====================

/**
 * 创建 Emby 用户并设置密码和策略
 */
async function createEmbyUserWithPolicy(username: string, password: string): Promise<string> {
  const { EmbyClient } = await import('@/lib/emby')

  // 1. 创建 Emby 用户
  const embyUser = await embyClient.createUser({
    Name: username,
    Password: password,
  })

  // 2. 设置用户密码（Emby API 创建用户时不会设置密码）
  await embyClient.setUserPassword(embyUser.Id, password)

  // 3. 应用默认用户策略
  await embyClient.setUserPolicy(embyUser.Id, EmbyClient.getDefaultPolicy())

  return embyUser.Id
}

/**
 * 保存用户注册信息到数据库（含邀请码更新和日志记录）
 */
async function saveRegistrationToDatabase(
  data: {
    username: string
    email: string
    inviteCode: string
  },
  embyUserId: string,
  invite: Invite
) {
  return prisma.$transaction(async (tx) => {
    // 1. 原子操作：更新邀请码使用次数（防止并发超额使用）
    const updateResult = await tx.invite.updateMany({
      where: {
        code: data.inviteCode,
        usedCount: {
          lt: invite.maxUses, // 确保还有剩余次数
        },
        // 如果有过期时间，确保未过期
        ...(invite.expiresAt && {
          expiresAt: {
            gte: new Date(),
          },
        }),
      },
      data: {
        usedCount: {
          increment: 1,
        },
      },
    })

    // 2. 检查是否更新成功（并发情况下可能失败）
    if (updateResult.count === 0) {
      throw new Error('邀请码已用尽或已过期（并发冲突）')
    }

    // 3. 创建用户记录
    const newUser = await tx.user.create({
      data: {
        username: data.username,
        email: data.email,
        embyId: embyUserId,
        inviteCode: data.inviteCode,
        expiresAt: calculateUserExpiry(new Date(), invite.defaultDays),
      },
    })

    // 4. 记录日志
    await createUserLog(tx, 'create_user', newUser.id, {
      username: newUser.username,
      email: newUser.email,
      embyId: newUser.embyId,
      inviteCode: data.inviteCode,
      expiresAt: newUser.expiresAt?.toISOString(),
      embyPolicy: 'default',
    })

    return newUser
  })
}

/**
 * 清理孤儿 Emby 用户（注册失败时的补偿逻辑）
 */
async function cleanupOrphanEmbyUser(embyUserId: string, username: string, error: unknown) {
  try {
    console.log(`[Cleanup] 删除 Emby 孤儿用户: ${embyUserId}`)
    await embyClient.deleteUser(embyUserId)
    console.log(`[Cleanup] Emby 用户清理成功: ${embyUserId}`)
  } catch (cleanupError) {
    // 清理失败，记录错误日志（需要人工介入）
    console.error(`[Cleanup] 清理 Emby 孤儿用户失败: ${embyUserId}`, cleanupError)

    // 尝试记录到数据库（如果数据库可用）
    try {
      await prisma.log.create({
        data: {
          action: 'orphan_emby_user',
          targetId: embyUserId,
          details: {
            username,
            embyId: embyUserId,
            error: '注册失败后无法清理 Emby 用户，需要人工删除',
            originalError: error instanceof Error ? error.message : String(error),
          },
        },
      })
    } catch {
      // 数据库也不可用，只能打日志
      console.error(`[Cleanup] 无法记录孤儿用户日志，请手动检查 Emby 用户: ${embyUserId}`)
    }
  }
}

// ==================== 用户延期相关辅助函数 ====================

/**
 * 更新用户到期时间到数据库（含日志记录）
 */
async function updateUserExpiryInDatabase(
  userId: string,
  username: string,
  oldExpiry: Date | null,
  newExpiry: Date,
  days: number,
  reactivate: boolean
) {
  await prisma.$transaction(async (tx) => {
    // 1. 更新到期时间（如果需要则同时更新状态）
    await tx.user.update({
      where: { id: userId },
      data: {
        expiresAt: newExpiry,
        ...(reactivate && { isActive: true }),
      },
    })

    // 2. 记录日志
    await createUserLog(tx, 'extend_expiry', userId, {
      username,
      days,
      oldExpiresAt: oldExpiry?.toISOString(),
      newExpiresAt: newExpiry.toISOString(),
      reactivated: reactivate,
    })
  })
}

/**
 * 回滚 Emby 重新启用操作（如果数据库更新失败）
 */
async function rollbackEmbyReactivation(embyId: string, userId: string, username: string) {
  try {
    // 回滚 Emby 状态到禁用
    await embyClient.setUserPolicy(embyId, {
      IsDisabled: true,
    })
    console.log(`[Rollback] Emby 状态已回滚（重新禁用）: ${embyId}`)
  } catch (rollbackError) {
    // 回滚失败，记录日志
    console.error(`[Rollback] Emby 状态回滚失败: ${embyId}`, rollbackError)

    try {
      await prisma.log.create({
        data: {
          action: 'sync_conflict',
          targetId: userId,
          details: {
            username,
            embyId,
            error: 'Emby 已重新启用但数据库更新失败，且 Emby 回滚失败',
            embyStatus: 'enabled',
            dbStatus: 'disabled',
          },
        },
      })
    } catch {
      console.error(`[Rollback] 无法记录同步冲突日志，请手动检查用户: ${userId}`)
    }
  }
}

// ==================== 用户状态切换相关辅助函数 ====================

/**
 * 更新用户状态到数据库（含日志记录）
 */
async function updateUserStatusInDatabase(
  userId: string,
  username: string,
  embyId: string,
  newStatus: boolean,
  action: string
) {
  await prisma.$transaction(async (tx) => {
    // 1. 更新用户状态
    await tx.user.update({
      where: { id: userId },
      data: {
        isActive: newStatus,
      },
    })

    // 2. 记录日志
    await createUserLog(tx, action, userId, {
      username,
      embyId,
    })
  })
}

/**
 * 回滚 Emby 状态（如果数据库更新失败）
 */
async function rollbackEmbyStatus(
  embyId: string,
  userId: string,
  username: string,
  originalStatus: boolean,
  targetStatus: boolean
) {
  try {
    // 回滚 Emby 状态到原始值
    await embyClient.setUserPolicy(embyId, {
      IsDisabled: !originalStatus, // 恢复原状态
    })
    console.log(`[Rollback] Emby 状态已回滚: ${embyId}`)
  } catch (rollbackError) {
    // 回滚失败，记录日志
    console.error(`[Rollback] Emby 状态回滚失败: ${embyId}`, rollbackError)

    try {
      await prisma.log.create({
        data: {
          action: 'sync_conflict',
          targetId: userId,
          details: {
            username,
            embyId,
            error: 'Emby 状态已更新但数据库更新失败，且 Emby 回滚失败',
            embyStatus: targetStatus ? 'enabled' : 'disabled',
            dbStatus: originalStatus ? 'active' : 'inactive',
          },
        },
      })
    } catch {
      console.error(`[Rollback] 无法记录同步冲突日志，请手动检查用户: ${userId}`)
    }
  }
}

// ==================== 用户删除相关辅助函数 ====================

/**
 * 从数据库删除用户（含日志记录）
 */
async function deleteUserFromDatabase(
  userId: string,
  username: string,
  email: string,
  embyId: string
) {
  await prisma.$transaction(async (tx) => {
    // 1. 删除用户记录
    await tx.user.delete({
      where: { id: userId },
    })

    // 2. 记录日志
    await createUserLog(tx, 'delete_user', userId, {
      username,
      email,
      embyId,
    })
  })
}

/**
 * 记录孤儿数据库记录（Emby 已删除但数据库删除失败）
 */
async function logOrphanDatabaseRecord(
  userId: string,
  username: string,
  embyId: string,
  error: unknown
) {
  try {
    await prisma.log.create({
      data: {
        action: 'orphan_db_record',
        targetId: userId,
        details: {
          username,
          embyId,
          error: 'Emby 用户已删除，但数据库记录删除失败，需要人工清理',
          originalError: error instanceof Error ? error.message : String(error),
        },
      },
    })
  } catch {
    console.error(`[Cleanup] 无法记录孤儿数据库记录，请手动检查用户: ${userId}`)
  }
}

// ==================== 导出的 Server Actions ====================

/**
 * 用户注册
 * @param data 注册信息
 * @returns { success: boolean, user?: User, error?: string }
 */
export async function registerUser(data: {
  username: string
  password: string
  email: string
  inviteCode: string
}) {
  let embyUserId: string | null = null

  try {
    // 1. 验证邀请码
    const inviteValidation = await validateInvite(data.inviteCode)
    if (!inviteValidation.success || !inviteValidation.invite) {
      return {
        success: false,
        error: inviteValidation.error || '邀请码无效',
      }
    }

    // 2. 检查用户名是否已存在
    const existingUser = await prisma.user.findUnique({
      where: { username: data.username },
    })

    if (existingUser) {
      return {
        success: false,
        error: '用户名已存在',
      }
    }

    // 3. 创建 Emby 用户并设置策略
    embyUserId = await createEmbyUserWithPolicy(data.username, data.password)

    // 4. 保存到数据库（含邀请码更新和日志）
    const user = await saveRegistrationToDatabase(
      data,
      embyUserId,
      inviteValidation.invite
    )

    // 5. 发送 Telegram 通知（异步，不阻塞返回）
    notifyNewRegistration({
      username: user.username,
      email: user.email,
      inviteCode: data.inviteCode,
      expiresAt: user.expiresAt,
      createdAt: user.createdAt,
    }).catch((error) => {
      console.error('[注册] Telegram 通知失败', error)
    })

    return {
      success: true,
      user,
      embyUrl: process.env.NEXT_PUBLIC_EMBY_URL || process.env.EMBY_URL,
    }
  } catch (error) {
    console.error('用户注册失败：', error)

    // 补偿逻辑：清理孤儿 Emby 用户
    if (embyUserId) {
      await cleanupOrphanEmbyUser(embyUserId, data.username, error)
    }

    return {
      success: false,
      error: formatEmbyError(error),
    }
  }
}

/**
 * 获取用户列表
 * @param params 查询参数 { search?: string, page?: number, pageSize?: number }
 * @returns 用户列表和总数
 */
export async function getUsers(params?: {
  search?: string
  page?: number
  pageSize?: number
}) {
  try {
    const page = params?.page || 1
    const pageSize = params?.pageSize || 10
    const skip = (page - 1) * pageSize

    const where = params?.search
      ? {
          username: {
            contains: params.search,
            mode: 'insensitive' as const,
          },
        }
      : undefined

    const [users, total] = await prisma.$transaction([
      prisma.user.findMany({
        where,
        include: {
          invite: true,
        },
        orderBy: {
          createdAt: 'desc',
        },
        skip,
        take: pageSize,
      }),
      prisma.user.count({ where }),
    ])

    return {
      success: true,
      users,
      total,
    }
  } catch (error) {
    console.error('获取用户列表失败：', error)
    return {
      success: false,
      error: '获取用户列表失败',
      users: [],
      total: 0,
    }
  }
}

/**
 * 延长到期时间
 * @param userId 用户 ID
 * @param days 延长天数
 * @returns { success: boolean, error?: string }
 */
export async function extendExpiry(userId: string, days: number) {
  try {
    // 1. 查找用户
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 2. 计算新的到期时间
    const newExpiry = calculateUserExpiry(user.expiresAt || new Date(), days)

    // 3. 判断是否需要重新启用
    const needsReactivation = shouldReactivateUser(user, newExpiry)

    // 4. 如果需要重新启用，先同步到 Emby
    if (needsReactivation) {
      try {
        await embyClient.setUserPolicy(user.embyId, {
          IsDisabled: false,
        })
      } catch (embyError) {
        console.error('Emby 重新启用失败：', embyError)
        return {
          success: false,
          error: formatEmbyError(embyError),
        }
      }
    }

    // 5. 更新数据库
    try {
      await updateUserExpiryInDatabase(
        userId,
        user.username,
        user.expiresAt,
        newExpiry,
        days,
        needsReactivation
      )

      return {
        success: true,
      }
    } catch (dbError) {
      // 数据库更新失败，如果已重新启用 Emby，尝试回滚
      console.error('数据库更新失败（尝试回滚 Emby）：', dbError)

      if (needsReactivation) {
        await rollbackEmbyReactivation(user.embyId, userId, user.username)
      }

      return {
        success: false,
        error: formatDatabaseError(dbError),
      }
    }
  } catch (error) {
    console.error('延长到期时间失败：', error)
    return {
      success: false,
      error: '延长到期时间失败，请稍后重试',
    }
  }
}

/**
 * 禁用/启用用户
 * @param userId 用户 ID
 * @returns { success: boolean, error?: string }
 */
export async function toggleUserStatus(userId: string) {
  try {
    // 1. 查找用户
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 2. 计算目标状态
    const { isActive: targetStatus, isDisabled: targetEmbyStatus } =
      getUserTargetStatus(user)

    // 3. 先同步到 Emby
    try {
      await embyClient.setUserPolicy(user.embyId, {
        IsDisabled: targetEmbyStatus,
      })
    } catch (embyError) {
      console.error('Emby 状态更新失败：', embyError)
      return {
        success: false,
        error: formatEmbyError(embyError),
      }
    }

    // 4. 更新数据库
    try {
      const action = user.isActive ? 'disable_user' : 'enable_user'
      await updateUserStatusInDatabase(
        userId,
        user.username,
        user.embyId,
        targetStatus,
        action
      )

      return {
        success: true,
      }
    } catch (dbError) {
      // 数据库更新失败，尝试回滚 Emby 状态
      console.error('数据库状态更新失败（尝试回滚 Emby）：', dbError)

      await rollbackEmbyStatus(
        user.embyId,
        userId,
        user.username,
        user.isActive,
        targetStatus
      )

      return {
        success: false,
        error: formatDatabaseError(dbError),
      }
    }
  } catch (error) {
    console.error('切换用户状态失败：', error)
    return {
      success: false,
      error: '操作失败，请稍后重试',
    }
  }
}

/**
 * 删除用户
 * @param userId 用户 ID
 * @returns { success: boolean, error?: string }
 */
export async function deleteUser(userId: string) {
  try {
    // 1. 查找用户
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 2. 先删除 Emby 用户
    try {
      await embyClient.deleteUser(user.embyId)
    } catch (embyError) {
      console.error('删除 Emby 用户失败：', embyError)
      return {
        success: false,
        error: formatEmbyError(embyError),
      }
    }

    // 3. Emby 删除成功后，再删除数据库记录
    try {
      await deleteUserFromDatabase(userId, user.username, user.email, user.embyId)

      return {
        success: true,
      }
    } catch (dbError) {
      // 数据库删除失败，但 Emby 已删除（留下孤儿数据库记录）
      console.error('删除数据库记录失败（Emby 用户已删除）：', dbError)

      await logOrphanDatabaseRecord(userId, user.username, user.embyId, dbError)

      return {
        success: false,
        error: '数据库记录删除失败，但 Emby 用户已删除，请联系管理员处理',
      }
    }
  } catch (error) {
    console.error('删除用户失败：', error)
    return {
      success: false,
      error: '删除用户失败，请稍后重试',
    }
  }
}

/**
 * 重置用户密码
 * @param userId 用户 ID
 * @returns { success: boolean, password?: string, error?: string }
 */
export async function resetPassword(userId: string) {
  try {
    // 1. 查找用户
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 2. 生成随机密码
    const newPassword = generateRandomPassword(12)

    // 3. 先更新 Emby 密码
    try {
      await embyClient.setUserPassword(user.embyId, newPassword)
    } catch (embyError) {
      console.error('Emby 密码重置失败：', embyError)
      return {
        success: false,
        error: formatEmbyError(embyError),
      }
    }

    // 4. 记录日志
    try {
      await prisma.$transaction(async (tx) => {
        await createUserLog(tx, 'reset_password', userId, {
          username: user.username,
          embyId: user.embyId,
        })
      })

      return {
        success: true,
        password: newPassword,
      }
    } catch (dbError) {
      console.error('记录日志失败：', dbError)
      return {
        success: false,
        error: formatDatabaseError(dbError),
      }
    }
  } catch (error) {
    console.error('重置密码失败：', error)
    return {
      success: false,
      error: '重置密码失败，请稍后重试',
    }
  }
}
