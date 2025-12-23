'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'
import { notifyExpiredUsers } from '@/lib/telegram'

/**
 * 检查并禁用过期用户
 * @returns { success: boolean, disabledCount: number, errors: string[] }
 */
export async function checkExpiredUsers() {
  try {
    const now = new Date()
    const errors: string[] = []
    let disabledCount = 0
    const disabledUsers: Array<{
      username: string
      email: string
      expiresAt: Date | null
    }> = []
    const failedUsers: Array<{ username: string; error: string }> = []

    // 查询已到期但仍启用的用户
    const expiredUsers = await prisma.user.findMany({
      where: {
        expiresAt: {
          lt: now,
        },
        isActive: true,
      },
    })

    console.log(`[Cron] 发现 ${expiredUsers.length} 个过期用户`)

    // 循环处理每个过期用户
    for (const user of expiredUsers) {
      try {
        // 1. 调用 Emby API 禁用用户
        await embyClient.setUserPolicy(user.embyId, {
          IsDisabled: true,
        })

        // 2. 更新数据库状态
        await prisma.user.update({
          where: { id: user.id },
          data: {
            isActive: false,
          },
        })

        // 3. 记录日志
        await prisma.log.create({
          data: {
            action: 'auto_disable_user',
            targetId: user.id,
            details: {
              username: user.username,
              embyId: user.embyId,
              expiresAt: user.expiresAt?.toISOString(),
              reason: 'expired',
            },
          },
        })

        disabledCount++
        disabledUsers.push({
          username: user.username,
          email: user.email,
          expiresAt: user.expiresAt,
        })
        console.log(`[Cron] 已禁用用户: ${user.username} (${user.id})`)
      } catch (error) {
        // 单个用户失败不影响其他用户
        const errorMsg = `${
          error instanceof Error ? error.message : String(error)
        }`
        errors.push(`禁用用户 ${user.username} 失败: ${errorMsg}`)
        failedUsers.push({
          username: user.username,
          error: errorMsg,
        })
        console.error(`[Cron] 禁用用户 ${user.username} 失败: ${errorMsg}`)

        // 记录错误日志
        await prisma.log.create({
          data: {
            action: 'auto_disable_user_failed',
            targetId: user.id,
            details: {
              username: user.username,
              embyId: user.embyId,
              error: errorMsg,
            },
          },
        })
      }
    }

    console.log(
      `[Cron] 定时任务完成，已禁用 ${disabledCount}/${expiredUsers.length} 个用户`
    )

    // 发送 Telegram 通知（异步，不阻塞返回）
    notifyExpiredUsers({
      disabledUsers,
      failedUsers,
      totalExpired: expiredUsers.length,
    }).catch((error) => {
      console.error('[Cron] Telegram 通知失败', error)
    })

    return {
      success: true,
      disabledCount,
      totalExpired: expiredUsers.length,
      errors,
    }
  } catch (error) {
    console.error('[Cron] 定时任务失败:', error)
    return {
      success: false,
      disabledCount: 0,
      totalExpired: 0,
      errors: [error instanceof Error ? error.message : String(error)],
    }
  }
}
