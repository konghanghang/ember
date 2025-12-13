'use server'

import { prisma } from '@/lib/db'
import { getServerAuth } from '@/lib/auth'
import { MediaType, SubscriptionStatus } from '@prisma/client'

/**
 * 用户提交订阅
 * @param data { type, name, tmdbId, note }
 * @returns { success: boolean, error?: string }
 */
export async function createSubscription(data: {
  type: MediaType
  name: string
  tmdbId: string
  note?: string
}) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'user') {
      return {
        success: false,
        error: '未登录或权限不足',
      }
    }

    // 2. 验证必填字段
    if (!data.name || !data.tmdbId) {
      return {
        success: false,
        error: '影视名称和 TMDB ID 为必填项',
      }
    }

    // 3. 创建订阅
    await prisma.subscription.create({
      data: {
        userId: auth.id,
        type: data.type,
        name: data.name,
        tmdbId: data.tmdbId,
        note: data.note,
        status: 'PENDING',
      },
    })

    // 4. 记录日志
    await prisma.log.create({
      data: {
        action: 'create_subscription',
        targetId: auth.id,
        details: {
          type: data.type,
          name: data.name,
          tmdbId: data.tmdbId,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('创建订阅失败：', error)
    return {
      success: false,
      error: '创建订阅失败，请稍后重试',
    }
  }
}

/**
 * 获取用户的订阅列表
 * @returns 订阅列表
 */
export async function getUserSubscriptions() {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'user') {
      return null
    }

    // 2. 查询订阅列表
    const subscriptions = await prisma.subscription.findMany({
      where: {
        userId: auth.id,
      },
      orderBy: {
        createdAt: 'desc',
      },
    })

    return subscriptions
  } catch (error) {
    console.error('获取订阅列表失败：', error)
    return null
  }
}

/**
 * 删除订阅（仅允许删除 PENDING 状态的订阅）
 * @param id 订阅 ID
 * @returns { success: boolean, error?: string }
 */
export async function deleteSubscription(id: string) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'user') {
      return {
        success: false,
        error: '未登录或权限不足',
      }
    }

    // 2. 查询订阅
    const subscription = await prisma.subscription.findUnique({
      where: { id },
    })

    if (!subscription) {
      return {
        success: false,
        error: '订阅不存在',
      }
    }

    // 3. 验证所有权
    if (subscription.userId !== auth.id) {
      return {
        success: false,
        error: '无权删除此订阅',
      }
    }

    // 4. 验证状态（只能删除 PENDING 状态）
    if (subscription.status !== 'PENDING') {
      return {
        success: false,
        error: '只能删除待审核的订阅',
      }
    }

    // 5. 删除订阅
    await prisma.subscription.delete({
      where: { id },
    })

    // 6. 记录日志
    await prisma.log.create({
      data: {
        action: 'delete_subscription',
        targetId: auth.id,
        details: {
          subscriptionId: id,
          name: subscription.name,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('删除订阅失败：', error)
    return {
      success: false,
      error: '删除订阅失败，请稍后重试',
    }
  }
}

/**
 * 获取所有订阅（管理员）
 * @param status 可选的状态筛选
 * @returns 订阅列表
 */
export async function getAllSubscriptions(status?: SubscriptionStatus) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'admin') {
      return null
    }

    // 2. 查询订阅列表
    const subscriptions = await prisma.subscription.findMany({
      where: status ? { status } : undefined,
      include: {
        user: {
          select: {
            username: true,
            email: true,
          },
        },
      },
      orderBy: {
        createdAt: 'desc',
      },
    })

    return subscriptions
  } catch (error) {
    console.error('获取订阅列表失败：', error)
    return null
  }
}

/**
 * 审核通过订阅（管理员）
 * @param id 订阅 ID
 * @returns { success: boolean, error?: string }
 */
export async function approveSubscription(id: string) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'admin') {
      return {
        success: false,
        error: '未登录或权限不足',
      }
    }

    // 2. 查询订阅
    const subscription = await prisma.subscription.findUnique({
      where: { id },
    })

    if (!subscription) {
      return {
        success: false,
        error: '订阅不存在',
      }
    }

    // 3. 验证状态（只能审核 PENDING 状态）
    if (subscription.status !== 'PENDING') {
      return {
        success: false,
        error: '订阅已被处理',
      }
    }

    // 4. 更新状态为 APPROVED（暂不调用 MoviePilot API）
    await prisma.subscription.update({
      where: { id },
      data: {
        status: 'APPROVED',
      },
    })

    // 5. 记录日志
    await prisma.log.create({
      data: {
        action: 'approve_subscription',
        targetId: subscription.userId,
        details: {
          subscriptionId: id,
          name: subscription.name,
          adminUsername: auth.username,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('审核订阅失败：', error)
    return {
      success: false,
      error: '审核订阅失败，请稍后重试',
    }
  }
}

/**
 * 拒绝订阅（管理员）
 * @param id 订阅 ID
 * @returns { success: boolean, error?: string }
 */
export async function rejectSubscription(id: string) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'admin') {
      return {
        success: false,
        error: '未登录或权限不足',
      }
    }

    // 2. 查询订阅
    const subscription = await prisma.subscription.findUnique({
      where: { id },
    })

    if (!subscription) {
      return {
        success: false,
        error: '订阅不存在',
      }
    }

    // 3. 验证状态（只能审核 PENDING 状态）
    if (subscription.status !== 'PENDING') {
      return {
        success: false,
        error: '订阅已被处理',
      }
    }

    // 4. 更新状态为 REJECTED
    await prisma.subscription.update({
      where: { id },
      data: {
        status: 'REJECTED',
      },
    })

    // 5. 记录日志
    await prisma.log.create({
      data: {
        action: 'reject_subscription',
        targetId: subscription.userId,
        details: {
          subscriptionId: id,
          name: subscription.name,
          adminUsername: auth.username,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('拒绝订阅失败：', error)
    return {
      success: false,
      error: '拒绝订阅失败，请稍后重试',
    }
  }
}
