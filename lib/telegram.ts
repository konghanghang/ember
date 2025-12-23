/**
 * Telegram 通知工具
 *
 * 用于向管理员发送订阅请求通知
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
