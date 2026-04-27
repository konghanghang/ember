export type DateTimeStyle = 'short' | 'long' | 'date' | 'time' | 'relative'

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
