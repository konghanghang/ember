import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * Tailwind CSS 类名合并工具
 * 用于 shadcn/ui 组件
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * 生成随机邀请码
 * @param length 邀请码长度（默认 8 位）
 * @returns 随机邀请码（大小写字母 + 数字）
 *
 * @example
 * generateInviteCode(8) // => "a7K9bX2Q"
 * generateInviteCode(6) // => "f3Hk7M"
 */
export function generateInviteCode(length: number = 8): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let code = ''

  for (let i = 0; i < length; i++) {
    const randomIndex = Math.floor(Math.random() * chars.length)
    code += chars[randomIndex]
  }

  return code
}

/**
 * 格式化日期时间
 * @param date Date 对象或 ISO 字符串
 * @returns 格式化后的日期时间字符串
 *
 * @example
 * formatDateTime(new Date()) // => "2024-12-06 22:30:15"
 */
export function formatDateTime(date: Date | string): string {
  const d = typeof date === 'string' ? new Date(date) : date

  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  const seconds = String(d.getSeconds()).padStart(2, '0')

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

/**
 * 格式化日期（仅日期）
 * @param date Date 对象或 ISO 字符串
 * @returns 格式化后的日期字符串
 *
 * @example
 * formatDate(new Date()) // => "2024-12-06"
 */
export function formatDate(date: Date | string): string {
  const d = typeof date === 'string' ? new Date(date) : date

  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')

  return `${year}-${month}-${day}`
}

/**
 * 计算剩余天数
 * @param expiresAt 到期时间
 * @returns 剩余天数（负数表示已过期）
 *
 * @example
 * getRemainingDays(new Date('2024-12-31')) // => 25
 * getRemainingDays(new Date('2024-12-01')) // => -5
 */
export function getRemainingDays(expiresAt: Date | string | null): number | null {
  if (!expiresAt) return null

  const expiry = typeof expiresAt === 'string' ? new Date(expiresAt) : expiresAt
  const now = new Date()

  const diffMs = expiry.getTime() - now.getTime()
  const diffDays = Math.ceil(diffMs / (1000 * 60 * 60 * 24))

  return diffDays
}

/**
 * 格式化剩余天数为友好提示
 * @param expiresAt 到期时间
 * @returns 友好提示字符串
 *
 * @example
 * formatRemainingDays(new Date('2024-12-31')) // => "剩余 25 天"
 * formatRemainingDays(new Date('2024-12-01')) // => "已过期 5 天"
 * formatRemainingDays(null) // => "永久有效"
 */
export function formatRemainingDays(expiresAt: Date | string | null): string {
  const days = getRemainingDays(expiresAt)

  if (days === null) return '永久有效'
  if (days > 0) return `剩余 ${days} 天`
  if (days === 0) return '今天到期'
  return `已过期 ${Math.abs(days)} 天`
}
