/**
 * 媒体缺口搜索候选的文件大小展示（MediaGapsView 搜索弹窗）。
 * - 字符串且非纯数值（如站点返回的 "1.2 GB" 标签）去空白后原样透传；
 * - 数值按 1024 进制自适应 B/KB/MB/GB/TB，精度随量级取 0/1/2 位小数；
 * - 非法输入（null/NaN/非正数/空串）返回 undefined，由调用方过滤不展示。
 */
export function formatGapCandidateSize(value: unknown): string | undefined {
  if (value === null || value === undefined) return undefined

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return undefined

    const numeric = Number(trimmed)
    if (!Number.isFinite(numeric)) {
      return trimmed
    }

    value = numeric
  }

  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return undefined
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }

  const precision = size >= 100 || unitIndex === 0 ? 0 : size >= 10 ? 1 : 2
  return `${size.toFixed(precision)} ${units[unitIndex]}`
}

/**
 * 订阅人工候选的文件大小展示（SubscriptionsView 候选卡片）。
 * 仅接受字节数：>=1GB 保留 1 位小数的 GB，>=1MB 保留 1 位小数的 MB，否则原样字节数；
 * 空/非正数返回空串。
 * 精度与单位覆盖和 formatGapCandidateSize 不同，属两处卡片各自的既有展示口径，
 * 强行合并会改变用户可见格式，故保留两个具名函数。
 */
export function formatSubscriptionCandidateSize(value?: number): string {
  if (!value || value <= 0) return ''
  if (value >= 1024 * 1024 * 1024) {
    return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`
  }
  if (value >= 1024 * 1024) {
    return `${(value / 1024 / 1024).toFixed(1)} MB`
  }
  return `${value} B`
}
