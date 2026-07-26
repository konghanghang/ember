import { describe, expect, it } from 'vitest'

import { formatGapCandidateSize, formatSubscriptionCandidateSize } from './format'

describe('formatGapCandidateSize（媒体缺口候选）', () => {
  it('空值与非法输入返回 undefined', () => {
    expect(formatGapCandidateSize(null)).toBeUndefined()
    expect(formatGapCandidateSize(undefined)).toBeUndefined()
    expect(formatGapCandidateSize('')).toBeUndefined()
    expect(formatGapCandidateSize('   ')).toBeUndefined()
    expect(formatGapCandidateSize(0)).toBeUndefined()
    expect(formatGapCandidateSize(-5)).toBeUndefined()
    expect(formatGapCandidateSize(Number.NaN)).toBeUndefined()
    expect(formatGapCandidateSize(Infinity)).toBeUndefined()
  })

  it('非数值字符串去空白后原样透传', () => {
    expect(formatGapCandidateSize('1.2 GB')).toBe('1.2 GB')
    expect(formatGapCandidateSize('  3.5GB  ')).toBe('3.5GB')
  })

  it('数值字符串按字节数换算', () => {
    expect(formatGapCandidateSize('512')).toBe('512 B')
    expect(formatGapCandidateSize('2048')).toBe('2.00 KB')
  })

  it('按 1024 进制进位并随量级调整精度', () => {
    expect(formatGapCandidateSize(512)).toBe('512 B')
    expect(formatGapCandidateSize(1536)).toBe('1.50 KB')
    expect(formatGapCandidateSize(10 * 1024 * 1024)).toBe('10.0 MB')
    expect(formatGapCandidateSize(100 * 1024 * 1024)).toBe('100 MB')
    expect(formatGapCandidateSize(5.5 * 1024 * 1024 * 1024)).toBe('5.50 GB')
    expect(formatGapCandidateSize(2 * 1024 ** 4)).toBe('2.00 TB')
  })

  it('非字符串非数字类型返回 undefined', () => {
    expect(formatGapCandidateSize({})).toBeUndefined()
    expect(formatGapCandidateSize([])).toBeUndefined()
    expect(formatGapCandidateSize(true)).toBeUndefined()
  })
})

describe('formatSubscriptionCandidateSize（订阅人工候选）', () => {
  it('空值与非正数返回空串', () => {
    expect(formatSubscriptionCandidateSize(undefined)).toBe('')
    expect(formatSubscriptionCandidateSize(0)).toBe('')
    expect(formatSubscriptionCandidateSize(-1)).toBe('')
  })

  it('不足 1MB 原样展示字节数', () => {
    expect(formatSubscriptionCandidateSize(500)).toBe('500 B')
    expect(formatSubscriptionCandidateSize(1024 * 1024 - 1)).toBe('1048575 B')
  })

  it('MB/GB 固定 1 位小数', () => {
    expect(formatSubscriptionCandidateSize(1024 * 1024)).toBe('1.0 MB')
    expect(formatSubscriptionCandidateSize(500 * 1024 * 1024)).toBe('500.0 MB')
    expect(formatSubscriptionCandidateSize(1.5 * 1024 * 1024 * 1024)).toBe('1.5 GB')
  })
})
