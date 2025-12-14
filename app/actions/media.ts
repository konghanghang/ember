'use server'

import { embyClient, type MediaStats } from '@/lib/emby'

// 缓存配置
const CACHE_DURATION = 5 * 60 * 1000 // 5分钟
let cachedStats: MediaStats | null = null
let cacheTimestamp: number = 0

/**
 * 获取媒体库统计信息（带缓存）
 * @returns 媒体库统计数据或错误
 */
export async function getMediaStats(): Promise<{
  success: boolean
  data?: MediaStats
  error?: string
}> {
  try {
    // 检查缓存
    const now = Date.now()
    if (cachedStats && now - cacheTimestamp < CACHE_DURATION) {
      return {
        success: true,
        data: cachedStats,
      }
    }

    // 调用 Emby API
    const stats = await embyClient.getMediaStats()

    // 更新缓存
    cachedStats = stats
    cacheTimestamp = now

    return {
      success: true,
      data: stats,
    }
  } catch (error) {
    console.error('获取媒体库统计失败:', error)
    return {
      success: false,
      error: error instanceof Error ? error.message : '获取媒体库统计失败',
    }
  }
}
