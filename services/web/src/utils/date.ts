export function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

export function formatPlaybackDate(dateString: string): string {
  const raw = String(dateString || '').trim()
  if (!raw) return '-'

  const isoWithTimezone = raw.match(
    /^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})(?::\d{2})?(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/
  )
  if (isoWithTimezone) {
    const [, year, month, day, hour, minute] = isoWithTimezone
    return `${year}/${month}/${day} ${hour}:${minute}`
  }

  const plainDateTime = raw.match(/^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})/)
  if (plainDateTime) {
    const [, year, month, day, hour, minute] = plainDateTime
    return `${year}/${month}/${day} ${hour}:${minute}`
  }

  return formatDate(raw)
}
