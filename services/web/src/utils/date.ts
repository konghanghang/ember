export type DateTimeStyle = 'short' | 'long' | 'date' | 'time' | 'time-seconds' | 'relative'

function parseDateValue(value: string | number | Date | null | undefined): Date | null {
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? null : value
  }

  if (typeof value === 'number') {
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? null : date
  }

  const raw = String(value || '').trim()
  if (!raw) {
    return null
  }

  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? null : date
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

function formatAbsolute(date: Date, style: Exclude<DateTimeStyle, 'relative'>): string {
  const year = date.getFullYear()
  const month = pad(date.getMonth() + 1)
  const day = pad(date.getDate())
  const hours = pad(date.getHours())
  const minutes = pad(date.getMinutes())
  const seconds = pad(date.getSeconds())

  switch (style) {
    case 'date':
      return `${year}-${month}-${day}`
    case 'time':
      return `${hours}:${minutes}`
    case 'time-seconds':
      return `${hours}:${minutes}:${seconds}`
    case 'long':
      return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
    case 'short':
    default:
      return `${year}-${month}-${day} ${hours}:${minutes}`
  }
}

function formatRelative(date: Date): string {
  const diffMs = date.getTime() - Date.now()
  const absMs = Math.abs(diffMs)
  const absMinutes = Math.round(absMs / (60 * 1000))

  if (absMinutes < 1) {
    return '刚刚'
  }
  if (absMinutes < 60) {
    return diffMs >= 0 ? `${absMinutes} 分钟后` : `${absMinutes} 分钟前`
  }

  const absHours = Math.round(absMs / (60 * 60 * 1000))
  if (absHours < 24) {
    return diffMs >= 0 ? `${absHours} 小时后` : `${absHours} 小时前`
  }

  const absDays = Math.round(absMs / (24 * 60 * 60 * 1000))
  if (absDays < 30) {
    return diffMs >= 0 ? `${absDays} 天后` : `${absDays} 天前`
  }

  return formatAbsolute(date, 'date')
}

export function formatDateTime(
  value: string | number | Date | null | undefined,
  style: DateTimeStyle = 'short',
  fallback = '-'
): string {
  const date = parseDateValue(value)
  if (!date) {
    return fallback
  }

  if (style === 'relative') {
    return formatRelative(date)
  }

  return formatAbsolute(date, style)
}

export function formatDate(dateString: string): string {
  return formatDateTime(dateString, 'short')
}

export function formatDateOnly(dateString: string | null | undefined, fallback = '-'): string {
  return formatDateTime(dateString, 'date', fallback)
}

export function formatTimeOnly(dateString: string | null | undefined, fallback = '-'): string {
  return formatDateTime(dateString, 'time', fallback)
}

export function formatPlaybackDate(dateString: string): string {
  const raw = String(dateString || '').trim()
  if (!raw) return '-'
  return formatDateTime(raw, 'short')
}

/** 本地时分秒（HH:mm:ss），用于"最后刷新时间"这类需要秒级精度的展示。 */
export function formatTimeWithSeconds(
  value: string | number | Date | null | undefined,
  fallback = '-'
): string {
  return formatDateTime(value, 'time-seconds', fallback)
}

/**
 * 点分日期（YYYY.MM.DD），SubscriptionsView 卡片既有展示口径。
 * 空值返回空串；非法日期原样返回输入字符串，保持历史展示行为，勿改为 fallback。
 */
export function formatDottedDate(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getFullYear()}.${pad(date.getMonth() + 1)}.${pad(date.getDate())}`
}

/**
 * 紧凑日期时间（MM-DD HH:mm），SubscriptionsView 卡片既有展示口径。
 * 空值返回空串；非法日期原样返回输入字符串，保持历史展示行为，勿改为 fallback。
 */
export function formatCompactDateTime(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

const SLASHED_DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})/
const SLASHED_DATETIME_PATTERN = /^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})/

/**
 * 斜杠日期（YYYY/MM/DD），MediaGapsView 既有展示口径。
 * 输入以 YYYY-MM-DD 开头时直接取字符串字面分量、不做时区换算（与历史 regex 实现一致，
 * 后端返回 UTC ISO 串时按原始墙钟时间展示）；其余可解析输入回退本地时区 short 格式。
 */
export function formatSlashedDate(value: string | null | undefined, fallback = '-'): string {
  const raw = String(value ?? '').trim()
  if (!raw) return fallback

  const match = raw.match(SLASHED_DATE_PATTERN)
  if (match) {
    return `${match[1]}/${match[2]}/${match[3]}`
  }

  return formatDateTime(raw, 'short', fallback)
}

/**
 * 斜杠日期时间（YYYY/MM/DD HH:mm），MediaGapsView 既有展示口径。
 * 时区语义同 formatSlashedDate：字面分量优先，不做换算。
 */
export function formatSlashedDateTime(value: string | null | undefined, fallback = '-'): string {
  const raw = String(value ?? '').trim()
  if (!raw) return fallback

  const match = raw.match(SLASHED_DATETIME_PATTERN)
  if (match) {
    return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
  }

  return formatDateTime(raw, 'short', fallback)
}

/** 本地日历日（YYYY-MM-DD），供按天对齐的接口参数（如 TV 日历查询区间）使用。 */
export function formatDateLocal(date: Date): string {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

/** 本地时区的本周周一 00:00（周一为一周起点，周日归上一周）。 */
export function startOfWeekLocal(date: Date): Date {
  const result = new Date(date)
  const weekday = result.getDay() === 0 ? 7 : result.getDay()
  result.setHours(0, 0, 0, 0)
  result.setDate(result.getDate() - weekday + 1)
  return result
}

/** 本地时区的当月 1 日 00:00。 */
export function startOfMonthLocal(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}

/** 本地时区的当月最后一日 00:00。 */
export function endOfMonthLocal(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth() + 1, 0)
}

/** 按日历日加减天数（setDate 语义，自动跨月/跨年），返回新 Date，不改原对象。 */
export function addDays(date: Date, days: number): Date {
  const result = new Date(date)
  result.setDate(result.getDate() + days)
  return result
}
