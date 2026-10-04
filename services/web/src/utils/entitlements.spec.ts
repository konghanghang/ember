import { describe, expect, it } from 'vitest'
import { benefitText, validateBenefits } from './entitlements'
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
