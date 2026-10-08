import type { WatchRetention } from '@/types/api'

/** 将服务端考核阶段和分组配置转换为一条观看要求，不在前端推算资格或首次日期。 */
export function watchRetentionText(rule: WatchRetention | undefined, timezone: string): string {
  if (!rule) return ''
  const base = `观看要求：最近 ${rule.days} 天累计观看至少 ${rule.minMinutes} 分钟`
  if (rule.state === 'waiting') return `观看要求：生效满 ${rule.days} 天后，每天检查最近 ${rule.days} 天是否累计观看至少 ${rule.minMinutes} 分钟。`
  if (rule.state === 'checking') return `${base}，每天检查。`
  if (rule.state === 'grace' && rule.firstCheckAt) {
    const date = new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, month: 'long', day: 'numeric' }).format(new Date(rule.firstCheckAt))
    return `${base}，首次检查为 ${date}。`
  }
  return `${base}。`
}

/** 与后端一致校验三项分组保号配置，不把未填写门槛静默转换为可用值。 */
export function validateWatchRetention(rule: { watchRetentionEnabled: boolean; watchRetentionDays: number; watchRetentionMinMinutes: number }): string {
  if (!Number.isInteger(rule.watchRetentionDays) || rule.watchRetentionDays < 1 || rule.watchRetentionDays > 3650) return '考核周期须为 1 至 3650 天'
  const minutes = rule.watchRetentionMinMinutes
  if (!Number.isInteger(minutes) || minutes < 0 || minutes > 5256000 || (rule.watchRetentionEnabled && minutes === 0)) return '开启观看保号时，请填写 1 至 5256000 的最低观看分钟数'
  return ''
}
