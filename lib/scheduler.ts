import cron from 'node-cron'
import { checkExpiredUsers } from '@/app/actions/cron'

/**
 * 定时任务调度器
 *
 * 使用 node-cron 在应用内部管理定时任务
 * 优点：
 * - 自包含，无需外部配置
 * - Docker 容器启动即生效
 * - 代码即配置
 */

let isSchedulerInitialized = false

/**
 * 初始化定时任务调度器
 */
export function initScheduler() {
  // 避免重复初始化
  if (isSchedulerInitialized) {
    console.log('[Scheduler] 已初始化，跳过')
    return
  }

  console.log('[Scheduler] 正在初始化定时任务...')

  // 从环境变量读取 cron 表达式，默认为每天凌晨 2:00
  const cronSchedule = process.env.CRON_SCHEDULE || '0 2 * * *'
  const timezone = process.env.CRON_TIMEZONE || 'Asia/Shanghai'

  console.log(`[Scheduler] Cron 配置: ${cronSchedule} (${timezone})`)

  // 执行用户过期检查
  cron.schedule(
    cronSchedule,
    async () => {
      console.log('[Scheduler] 开始执行定时任务: 检查过期用户')

      try {
        const result = await checkExpiredUsers()

        if (result.success) {
          console.log(
            `[Scheduler] 定时任务执行成功: 已处理 ${result.totalExpired} 个过期用户，成功禁用 ${result.disabledCount} 个`
          )
        } else {
          console.error('[Scheduler] 定时任务执行失败:', result.errors)
        }
      } catch (error) {
        console.error('[Scheduler] 定时任务执行异常:', error)
      }
    },
    {
      timezone,
    }
  )

  isSchedulerInitialized = true
  console.log(`[Scheduler] 定时任务已启动（${cronSchedule}）`)
}

/**
 * 手动触发定时任务（仅用于测试）
 */
export async function triggerManually() {
  console.log('[Scheduler] 手动触发定时任务')
  return await checkExpiredUsers()
}
