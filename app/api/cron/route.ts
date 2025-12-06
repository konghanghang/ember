import { NextRequest, NextResponse } from 'next/server'

// 使用 Node.js runtime（Prisma 需要）
export const runtime = 'nodejs'
// 强制动态渲染（避免构建时执行）
export const dynamic = 'force-dynamic'

/**
 * Cron API 路由
 * 用于 Vercel Cron 或手动触发定时任务
 *
 * Vercel Cron 配置（在 vercel.json 中）：
 * {
 *   "crons": [{
 *     "path": "/api/cron",
 *     "schedule": "0 2 * * *"
 *   }]
 * }
 */
export async function GET(request: NextRequest) {
  try {
    // 验证请求来源（可选）
    const authHeader = request.headers.get('authorization')
    const cronSecret = process.env.CRON_SECRET

    // 如果设置了 CRON_SECRET，则验证
    if (cronSecret && authHeader !== `Bearer ${cronSecret}`) {
      return NextResponse.json(
        { error: 'Unauthorized' },
        { status: 401 }
      )
    }

    // 动态导入（避免构建时执行）
    const { checkExpiredUsers } = await import('@/app/actions/cron')

    // 执行定时任务
    const result = await checkExpiredUsers()

    return NextResponse.json({
      success: result.success,
      message: `已处理 ${result.totalExpired} 个过期用户，成功禁用 ${result.disabledCount} 个`,
      data: {
        disabledCount: result.disabledCount,
        totalExpired: result.totalExpired,
        errors: result.errors,
      },
    })
  } catch (error) {
    console.error('[API] Cron 任务失败:', error)
    return NextResponse.json(
      {
        error: 'Internal Server Error',
        message: error instanceof Error ? error.message : String(error),
      },
      { status: 500 }
    )
  }
}
