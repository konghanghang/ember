'use server'

import { prisma } from '@/lib/db'
import { generateInviteCode } from '@/lib/utils'
import { canInviteBeUsed, canInviteBeDeleted } from '@/lib/invite-helpers'

/**
 * 生成邀请码
 * @param data 邀请码配置
 * @returns { success: boolean, invite?: Invite, error?: string }
 */
export async function createInvite(data: {
  maxUses?: number
  expiresAt?: Date
  defaultDays?: number
}) {
  try {
    // 1. 生成唯一邀请码
    let code = generateInviteCode(8)

    // 确保邀请码唯一（最多尝试 10 次）
    for (let i = 0; i < 10; i++) {
      const existing = await prisma.invite.findUnique({
        where: { code },
      })

      if (!existing) break
      code = generateInviteCode(8)
    }

    // 2. 创建邀请码
    const invite = await prisma.invite.create({
      data: {
        code,
        maxUses: data.maxUses || 1,
        expiresAt: data.expiresAt,
        defaultDays: data.defaultDays || 30,
      },
    })

    // 3. 记录日志
    await prisma.log.create({
      data: {
        action: 'create_invite',
        targetId: invite.id,
        details: {
          code: invite.code,
          maxUses: invite.maxUses,
          defaultDays: invite.defaultDays,
        },
      },
    })

    return {
      success: true,
      invite,
    }
  } catch (error) {
    console.error('创建邀请码失败：', error)
    return {
      success: false,
      error: '创建邀请码失败，请稍后重试',
    }
  }
}

/**
 * 获取邀请码列表
 * @returns 邀请码列表（含使用情况）
 */
export async function getInvites() {
  try {
    const invites = await prisma.invite.findMany({
      include: {
        users: {
          select: {
            id: true,
            username: true,
            createdAt: true,
          },
        },
      },
      orderBy: {
        createdAt: 'desc',
      },
    })

    return {
      success: true,
      invites,
    }
  } catch (error) {
    console.error('获取邀请码列表失败：', error)
    return {
      success: false,
      error: '获取邀请码列表失败',
      invites: [],
    }
  }
}

/**
 * 删除邀请码
 * @param id 邀请码 ID
 * @returns { success: boolean, error?: string }
 */
export async function deleteInvite(id: string) {
  try {
    // 1. 查找邀请码
    const invite = await prisma.invite.findUnique({
      where: { id },
    })

    if (!invite) {
      return {
        success: false,
        error: '邀请码不存在',
      }
    }

    // 2. 验证是否可以删除
    if (!canInviteBeDeleted(invite)) {
      return {
        success: false,
        error: '邀请码已被使用，无法删除',
      }
    }

    // 3. 删除邀请码
    await prisma.invite.delete({
      where: { id },
    })

    // 4. 记录日志
    await prisma.log.create({
      data: {
        action: 'delete_invite',
        targetId: id,
        details: {
          code: invite.code,
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('删除邀请码失败：', error)
    return {
      success: false,
      error: '删除邀请码失败，请稍后重试',
    }
  }
}

/**
 * 验证邀请码
 * @param code 邀请码
 * @returns { success: boolean, invite?: Invite, error?: string }
 */
export async function validateInvite(code: string) {
  try {
    // 1. 查找邀请码
    const invite = await prisma.invite.findUnique({
      where: { code },
    })

    if (!invite) {
      return {
        success: false,
        error: '邀请码不存在',
      }
    }

    // 2. 验证邀请码是否可用
    const validation = canInviteBeUsed(invite)
    if (!validation.valid) {
      return {
        success: false,
        error: validation.reason || '邀请码无效',
      }
    }

    return {
      success: true,
      invite,
    }
  } catch (error) {
    console.error('验证邀请码失败：', error)
    return {
      success: false,
      error: '验证邀请码失败，请稍后重试',
    }
  }
}
