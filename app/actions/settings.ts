'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'

/**
 * 获取系统信息
 * @returns 系统统计信息
 */
export async function getSystemInfo() {
  try {
    const [userCount, inviteCount, activeUserCount] = await Promise.all([
      prisma.user.count(),
      prisma.invite.count(),
      prisma.user.count({
        where: { isActive: true },
      }),
    ])

    return {
      success: true,
      info: {
        userCount,
        activeUserCount,
        inviteCount,
      },
    }
  } catch (error) {
    console.error('获取系统信息失败:', error)
    return {
      success: false,
      error: '获取系统信息失败',
    }
  }
}

/**
 * 测试 Emby 连接
 * @returns 连接状态
 */
export async function testEmbyConnection() {
  try {
    // 尝试获取用户列表来测试连接
    await embyClient.getUsers()

    return {
      success: true,
      message: 'Emby 服务器连接正常',
    }
  } catch (error) {
    console.error('Emby 连接测试失败:', error)
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Emby 服务器连接失败',
    }
  }
}
