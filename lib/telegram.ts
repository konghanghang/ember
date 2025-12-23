/**
 * Telegram 通知工具
 *
 * 用于向管理员发送订阅请求、用户注册和用户过期通知
 */

import { MediaType } from '@prisma/client'

/**
 * 检查 Telegram 是否已配置
 */
export function isTelegramConfigured(): boolean {
  const token = process.env.TELEGRAM_BOT_TOKEN
  const chatId = process.env.TELEGRAM_ADMIN_CHAT_ID
  return Boolean(token && chatId)
}

/**
 * 发送新订阅通知给管理员
 *
 * @param subscription 订阅数据
 * @param username 用户名
 */
export async function notifyNewSubscription(
  subscription: {
    type: MediaType
    name: string
    tmdbId: string
    posterPath?: string | null
    note?: string | null
    createdAt: Date
  },
  username: string
): Promise<void> {
  const token = process.env.TELEGRAM_BOT_TOKEN
  const chatId = process.env.TELEGRAM_ADMIN_CHAT_ID

  // 未配置时跳过通知
  if (!token || !chatId) {
    console.log('[Telegram] 未配置 Telegram，跳过通知')
    return
  }

  try {
    // 构建 TMDB 链接
    const tmdbUrl =
      subscription.type === 'MOVIE'
        ? `https://www.themoviedb.org/movie/${subscription.tmdbId}`
        : `https://www.themoviedb.org/tv/${subscription.tmdbId}`

    // 构建通知消息
    const message = `
🎬 新订阅请求

👤 用户：${username}
📺 影视：${subscription.name}
🎭 类型：${subscription.type === 'MOVIE' ? '电影' : '电视剧'}
🔗 TMDB：${tmdbUrl}
${subscription.note ? `💬 备注：${subscription.note}` : ''}
⏰ 时间：${subscription.createdAt.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}
    `.trim()

    // 如果有封面图，使用 sendPhoto；否则使用 sendMessage
    if (subscription.posterPath) {
      const posterUrl = `https://image.tmdb.org/t/p/w500${subscription.posterPath}`
      await sendTelegramPhoto(token, chatId, posterUrl, message)
    } else {
      await sendTelegramMessage(token, chatId, message)
    }

    console.log('[Telegram] 新订阅通知已发送', { username, name: subscription.name })
  } catch (error) {
    // 通知失败不影响主流程，仅记录日志
    console.error('[Telegram] 发送通知失败', error)
  }
}

/**
 * 发送新用户注册通知给管理员
 *
 * @param user 用户数据
 */
export async function notifyNewRegistration(user: {
  username: string
  email: string
  inviteCode: string
  expiresAt?: Date | null
  createdAt: Date
}): Promise<void> {
  const token = process.env.TELEGRAM_BOT_TOKEN
  const chatId = process.env.TELEGRAM_ADMIN_CHAT_ID

  // 未配置时跳过通知
  if (!token || !chatId) {
    console.log('[Telegram] 未配置 Telegram，跳过通知')
    return
  }

  try {
    // 格式化到期时间
    const expiryText = user.expiresAt
      ? user.expiresAt.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })
      : '永久'

    // 构建通知消息
    const message = `
👥 新用户注册

👤 用户名：${user.username}
📧 邮箱：${user.email}
🎫 邀请码：${user.inviteCode}
⏰ 注册时间：${user.createdAt.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}
📅 到期时间：${expiryText}
    `.trim()

    await sendTelegramMessage(token, chatId, message)

    console.log('[Telegram] 新用户注册通知已发送', { username: user.username })
  } catch (error) {
    // 通知失败不影响主流程，仅记录日志
    console.error('[Telegram] 发送通知失败', error)
  }
}

/**
 * 发送用户过期批量禁用通知
 *
 * @param result 定时任务执行结果
 */
export async function notifyExpiredUsers(result: {
  disabledUsers: Array<{
    username: string
    email: string
    expiresAt: Date | null
  }>
  failedUsers: Array<{
    username: string
    error: string
  }>
  totalExpired: number
}): Promise<void> {
  const token = process.env.TELEGRAM_BOT_TOKEN
  const chatId = process.env.TELEGRAM_ADMIN_CHAT_ID

  // 未配置或无过期用户时跳过通知
  if (!token || !chatId || result.totalExpired === 0) {
    if (result.totalExpired === 0) {
      console.log('[Telegram] 无过期用户，跳过通知')
    } else {
      console.log('[Telegram] 未配置 Telegram，跳过通知')
    }
    return
  }

  try {
    // 构建成功禁用的用户列表
    const disabledList =
      result.disabledUsers.length > 0
        ? result.disabledUsers
            .map((user) => {
              const expiry = user.expiresAt
                ? user.expiresAt.toLocaleDateString('zh-CN')
                : '未知'
              return `  • ${user.username} (${user.email}) - 过期时间: ${expiry}`
            })
            .join('\n')
        : '  无'

    // 构建失败的用户列表
    const failedList =
      result.failedUsers.length > 0
        ? result.failedUsers
            .map((user) => `  • ${user.username}: ${user.error}`)
            .join('\n')
        : '  无'

    // 构建通知消息
    const message = `
⏰ 定时任务：用户过期检查

📊 统计：
  • 发现过期用户：${result.totalExpired} 个
  • 成功禁用：${result.disabledUsers.length} 个
  • 失败：${result.failedUsers.length} 个

✅ 已禁用用户：
${disabledList}

${result.failedUsers.length > 0 ? `❌ 禁用失败：\n${failedList}` : ''}

🕐 执行时间：${new Date().toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}
    `.trim()

    await sendTelegramMessage(token, chatId, message)

    console.log('[Telegram] 用户过期通知已发送', {
      totalExpired: result.totalExpired,
      disabled: result.disabledUsers.length,
      failed: result.failedUsers.length,
    })
  } catch (error) {
    // 通知失败不影响主流程，仅记录日志
    console.error('[Telegram] 发送通知失败', error)
  }
}

/**
 * 发送纯文本消息
 */
async function sendTelegramMessage(
  token: string,
  chatId: string,
  text: string
): Promise<void> {
  const response = await fetch(
    `https://api.telegram.org/bot${token}/sendMessage`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        chat_id: chatId,
        text,
        parse_mode: 'HTML',
      }),
    }
  )

  if (!response.ok) {
    const error = await response.text()
    throw new Error(`Telegram API 错误: ${error}`)
  }
}

/**
 * 发送带图片的消息
 */
async function sendTelegramPhoto(
  token: string,
  chatId: string,
  photoUrl: string,
  caption: string
): Promise<void> {
  const response = await fetch(
    `https://api.telegram.org/bot${token}/sendPhoto`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        chat_id: chatId,
        photo: photoUrl,
        caption,
        parse_mode: 'HTML',
      }),
    }
  )

  if (!response.ok) {
    const error = await response.text()
    throw new Error(`Telegram API 错误: ${error}`)
  }
}
