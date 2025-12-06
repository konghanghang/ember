'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'
import { validateInvite } from './invites'
import { add } from 'date-fns'

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
  try {
    // 1. 验证邀请码
    const inviteValidation = await validateInvite(data.inviteCode)
    if (!inviteValidation.success || !inviteValidation.invite) {
      return {
        success: false,
        error: inviteValidation.error || '邀请码无效',
      }
    }

    const invite = inviteValidation.invite

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

    // 3. 使用事务：创建 Emby 用户 + 保存数据库
    const user = await prisma.$transaction(async (tx) => {
      // 3.1 创建 Emby 用户
      const embyUser = await embyClient.createUser({
        Name: data.username,
        Password: data.password,
      })

      // 3.2 保存到数据库
      const newUser = await tx.user.create({
        data: {
          username: data.username,
          email: data.email,
          embyId: embyUser.Id,
          inviteCode: data.inviteCode,
          expiresAt: add(new Date(), { days: invite.defaultDays }),
        },
      })

      // 3.3 更新邀请码使用次数
      await tx.invite.update({
        where: { code: data.inviteCode },
        data: {
          usedCount: {
            increment: 1,
          },
        },
      })

      // 3.4 记录日志
      await tx.log.create({
        data: {
          action: 'create_user',
          targetId: newUser.id,
          details: {
            username: newUser.username,
            email: newUser.email,
            embyId: newUser.embyId,
            inviteCode: data.inviteCode,
            expiresAt: newUser.expiresAt?.toISOString(),
          },
        },
      })

      return newUser
    })

    return {
      success: true,
      user,
    }
  } catch (error) {
    console.error('用户注册失败：', error)

    // 区分 Emby API 错误和其他错误
    const errorMessage =
      error instanceof Error && error.message.includes('Emby')
        ? 'Emby 用户创建失败，请检查 Emby 服务器状态'
        : '用户注册失败，请稍后重试'

    return {
      success: false,
      error: errorMessage,
    }
  }
}

/**
 * 获取用户列表
 * @param params 查询参数 { search?: string }
 * @returns 用户列表
 */
export async function getUsers(params?: { search?: string }) {
  try {
    const users = await prisma.user.findMany({
      where: params?.search
        ? {
            username: {
              contains: params.search,
              mode: 'insensitive',
            },
          }
        : undefined,
      include: {
        invite: true,
      },
      orderBy: {
        createdAt: 'desc',
      },
    })

    return {
      success: true,
      users,
    }
  } catch (error) {
    console.error('获取用户列表失败：', error)
    return {
      success: false,
      error: '获取用户列表失败',
      users: [],
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
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 计算新的到期时间
    const newExpiresAt = add(user.expiresAt || new Date(), { days })

    // 更新到期时间
    await prisma.user.update({
      where: { id: userId },
      data: {
        expiresAt: newExpiresAt,
      },
    })

    // 记录日志
    await prisma.log.create({
      data: {
        action: 'extend_expiry',
        targetId: userId,
        details: {
          username: user.username,
          days,
          oldExpiresAt: user.expiresAt?.toISOString(),
          newExpiresAt: newExpiresAt.toISOString(),
        },
      },
    })

    return {
      success: true,
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
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 同步到 Emby
    await embyClient.setUserPolicy(user.embyId, {
      IsDisabled: !user.isActive,
    })

    // 更新数据库
    await prisma.user.update({
      where: { id: userId },
      data: {
        isActive: !user.isActive,
      },
    })

    // 记录日志
    await prisma.log.create({
      data: {
        action: user.isActive ? 'disable_user' : 'enable_user',
        targetId: userId,
        details: {
          username: user.username,
          embyId: user.embyId,
        },
      },
    })

    return {
      success: true,
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
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 使用事务：删除 Emby 用户 + 删除数据库记录
    await prisma.$transaction(async (tx) => {
      // 删除 Emby 用户
      await embyClient.deleteUser(user.embyId)

      // 删除数据库记录
      await tx.user.delete({
        where: { id: userId },
      })

      // 记录日志
      await tx.log.create({
        data: {
          action: 'delete_user',
          targetId: userId,
          details: {
            username: user.username,
            email: user.email,
            embyId: user.embyId,
          },
        },
      })
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('删除用户失败：', error)
    return {
      success: false,
      error: '删除用户失败，请稍后重试',
    }
  }
}
