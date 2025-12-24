/**
 * Next.js Instrumentation
 *
 * 此文件在服务器启动时执行，用于：
 * - 初始化定时任务调度器
 * - 设置监控和追踪
 * - 其他服务器级别的初始化操作
 *
 * 文档：https://nextjs.org/docs/app/building-your-application/optimizing/instrumentation
 */

export async function register() {
  // 只在 Node.js 运行时执行（不在 Edge Runtime）
  if (process.env.NEXT_RUNTIME === 'nodejs') {
    const { initScheduler } = await import('./lib/scheduler')

    console.log('[Instrumentation] 服务器启动，正在初始化...')

    // 初始化定时任务调度器
    initScheduler()

    console.log('[Instrumentation] 初始化完成')
  }
}
