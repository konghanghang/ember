import type { Plan, PlanBenefit } from '@/types/api'

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

/** 展示商品的唯一分组和明确有效期，不从历史订单快照推断。 */
export function planEntitlementText(plan: Pick<Plan, 'planGroup' | 'planGroupName' | 'validityType' | 'days'>): string {
  return `${plan.planGroupName || plan.planGroup} · ${plan.validityType === 'permanent' ? '永久' : `${plan.days} 天`}`
}

/** 单组商品提交校验；永久天数由请求组装规范化为零。 */
export function validateSinglePlan(plan: Pick<Plan, 'planGroup' | 'validityType' | 'days'>): string {
  if (!plan.planGroup) return '请选择权益分组'
  if (plan.validityType !== 'duration' && plan.validityType !== 'permanent') return '请选择有效期'
  if (plan.validityType === 'duration' && (!Number.isInteger(plan.days) || plan.days < 1)) return '有效天数必须为正整数'
  return ''
}
