import { describe, expect, it } from 'vitest'
import { benefitText, validateBenefits, planEntitlementText, validateSinglePlan } from './entitlements'
import { formatDateTimeInTimezone } from './date'

describe('套餐权益', () => {
  it('到期时间按全局业务时区转换日期边界', () => {
    expect(formatDateTimeInTimezone('2026-10-04T18:00:00Z', 'Asia/Shanghai')).toContain('2026/10/05')
    expect(formatDateTimeInTimezone('2026-10-04T18:00:00Z', 'America/New_York')).toContain('2026/10/04')
  })
  it('明确区分永久与限时，并逐项展示组合套餐', () => {
    expect(benefitText({ planGroup: 'A', validityType: 'permanent' })).toBe('A · 永久')
    expect(benefitText({ planGroup: 'B', validityType: 'duration', durationDays: 30 })).toBe('B · 30 天')
  })
  it('拒绝空组合、重复分组和不明确的期限', () => {
    expect(validateBenefits([])).not.toBe('')
    expect(validateBenefits([{ planGroup: 'A', validityType: 'duration', durationDays: 0 }])).not.toBe('')
    expect(validateBenefits([{ planGroup: 'A', validityType: 'permanent' }, { planGroup: 'A', validityType: 'permanent' }])).not.toBe('')
    expect(validateBenefits([{ planGroup: 'A', validityType: 'permanent' }, { planGroup: 'B', validityType: 'duration', durationDays: 60 }])).toBe('')
  })
})

// 商品使用单组字段，订单历史和人工补偿仍可展示多项。
describe('单组商品展示与校验', () => {
  it('展示当前名称与明确期限', () => {
    expect(planEntitlementText({ planGroup: 'A', planGroupName: '基础', validityType: 'permanent', days: 0 })).toBe('基础 · 永久')
    expect(planEntitlementText({ planGroup: 'B', validityType: 'duration', days: 60 })).toBe('B · 60 天')
  })
  it('校验正整数天数，永久忽略编辑器残留天数', () => {
    expect(validateSinglePlan({ planGroup: 'A', validityType: 'permanent', days: 30 })).toBe('')
    for (const days of [0, -1, 1.5, NaN]) expect(validateSinglePlan({ planGroup: 'A', validityType: 'duration', days })).not.toBe('')
  })
})
