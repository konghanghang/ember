import type { PlanBenefit } from '@/types/api'

/** 展示单项权益，永久不使用 0 天或空日期代替。 */
export function benefitText(benefit: PlanBenefit, names: Record<string, string> = {}): string {
  return `${names[benefit.planGroup] || benefit.planGroupName || benefit.planGroup} · ${benefit.validityType === 'permanent' ? '永久' : `${benefit.durationDays} 天`}`
}

/** 提交前校验整组权益，后端仍执行相同的权威校验。 */
export function validateBenefits(benefits: PlanBenefit[]): string {
  if (!benefits.length) return '请添加至少一项权益'
  const seen = new Set<string>()
  for (const benefit of benefits) {
    if (!benefit.planGroup || seen.has(benefit.planGroup)) return '请选择分组，同一分组不能重复'
    seen.add(benefit.planGroup)
    if (benefit.validityType === 'duration' && (!Number.isInteger(benefit.durationDays) || (benefit.durationDays ?? 0) < 1)) return '限时权益天数必须为正整数'
    if (benefit.validityType === 'permanent' && benefit.durationDays) return '永久权益不填写天数'
  }
  return ''
}
