import { describe, expect, it } from 'vitest'

import {
  addDays,
  endOfMonthLocal,
  formatCompactDateTime,
  formatDateLocal,
  formatDottedDate,
  formatSlashedDate,
  formatSlashedDateTime,
  formatTimeWithSeconds,
  startOfMonthLocal,
  startOfWeekLocal
} from './date'

describe('formatTimeWithSeconds', () => {
  it('输出 HH:mm:ss 并补零', () => {
    expect(formatTimeWithSeconds(new Date(2026, 6, 26, 9, 5, 3))).toBe('09:05:03')
    expect(formatTimeWithSeconds(new Date(2026, 6, 26, 23, 59, 58))).toBe('23:59:58')
  })

  it('非法输入返回 fallback', () => {
    expect(formatTimeWithSeconds('not-a-date')).toBe('-')
    expect(formatTimeWithSeconds(null)).toBe('-')
    expect(formatTimeWithSeconds(undefined, '')).toBe('')
  })
})

describe('formatSlashedDate / formatSlashedDateTime', () => {
  it('YYYY-MM-DD 开头的输入取字面分量，不做时区换算', () => {
    expect(formatSlashedDate('2026-07-26')).toBe('2026/07/26')
    expect(formatSlashedDate('2026-07-26T23:30:00Z')).toBe('2026/07/26')
    expect(formatSlashedDateTime('2026-07-26T23:30:00Z')).toBe('2026/07/26 23:30')
    expect(formatSlashedDateTime('2026-07-26 08:05')).toBe('2026/07/26 08:05')
  })

  it('空输入返回 fallback', () => {
    expect(formatSlashedDate('')).toBe('-')
    expect(formatSlashedDate(undefined)).toBe('-')
    expect(formatSlashedDateTime(null)).toBe('-')
    expect(formatSlashedDate('  ', 'N/A')).toBe('N/A')
  })

  it('无法解析的输入走 short 兜底并返回 fallback', () => {
    expect(formatSlashedDate('garbage')).toBe('-')
    expect(formatSlashedDateTime('garbage')).toBe('-')
  })

  it('非标准但可解析的输入回退本地 short 格式', () => {
    const parsed = new Date('07/26/2026 10:05')
    if (Number.isNaN(parsed.getTime())) return // 引擎不支持该格式时跳过
    expect(formatSlashedDate('07/26/2026 10:05')).toBe('2026-07-26 10:05')
    expect(formatSlashedDateTime('07/26/2026 10:05')).toBe('2026-07-26 10:05')
  })
})

describe('formatDottedDate / formatCompactDateTime', () => {
  it('空值返回空串', () => {
    expect(formatDottedDate(null)).toBe('')
    expect(formatDottedDate(undefined)).toBe('')
    expect(formatDottedDate('')).toBe('')
    expect(formatCompactDateTime(null)).toBe('')
    expect(formatCompactDateTime('')).toBe('')
  })

  it('按既有口径格式化本地时间', () => {
    expect(formatDottedDate('2026-07-26T10:30:00')).toBe('2026.07.26')
    expect(formatDottedDate('2026-01-05T00:30:00')).toBe('2026.01.05')
    expect(formatCompactDateTime('2026-07-26T10:30:00')).toBe('07-26 10:30')
    expect(formatCompactDateTime('2026-12-31T09:05:00')).toBe('12-31 09:05')
  })

  it('非法日期原样返回输入字符串', () => {
    expect(formatDottedDate('上映在即')).toBe('上映在即')
    expect(formatCompactDateTime('未知时间')).toBe('未知时间')
  })
})

describe('formatDateLocal', () => {
  it('输出本地日历日并补零', () => {
    expect(formatDateLocal(new Date(2026, 6, 26, 15, 30))).toBe('2026-07-26')
    expect(formatDateLocal(new Date(2026, 0, 5))).toBe('2026-01-05')
  })
})

describe('startOfWeekLocal', () => {
  // 2026-07-20 是周一，2026-07-26 是周日
  it('周日归到上一周的周一', () => {
    const result = startOfWeekLocal(new Date(2026, 6, 26, 18, 45))
    expect(result.getFullYear()).toBe(2026)
    expect(result.getMonth()).toBe(6)
    expect(result.getDate()).toBe(20)
    expect(result.getHours()).toBe(0)
    expect(result.getMinutes()).toBe(0)
    expect(result.getSeconds()).toBe(0)
  })

  it('周三归到本周一', () => {
    const result = startOfWeekLocal(new Date(2026, 6, 22, 9, 0))
    expect(result.getDate()).toBe(20)
  })

  it('周一当天保持不变且归零时间', () => {
    const result = startOfWeekLocal(new Date(2026, 6, 20, 23, 59))
    expect(result.getDate()).toBe(20)
    expect(result.getHours()).toBe(0)
  })

  it('不修改入参对象', () => {
    const input = new Date(2026, 6, 22, 9, 0)
    startOfWeekLocal(input)
    expect(input.getDate()).toBe(22)
  })
})

describe('startOfMonthLocal / endOfMonthLocal', () => {
  it('返回当月首末日的 00:00', () => {
    expect(startOfMonthLocal(new Date(2026, 6, 26, 12, 0))).toEqual(new Date(2026, 6, 1))
    expect(endOfMonthLocal(new Date(2026, 6, 10))).toEqual(new Date(2026, 6, 31))
  })

  it('平年二月末日为 28 日', () => {
    expect(endOfMonthLocal(new Date(2026, 1, 10)).getDate()).toBe(28)
  })
})

describe('addDays', () => {
  it('跨月加减并保持时间分量', () => {
    expect(addDays(new Date(2026, 6, 31, 10, 30), 1)).toEqual(new Date(2026, 7, 1, 10, 30))
    expect(addDays(new Date(2026, 0, 1, 10, 30), -1)).toEqual(new Date(2025, 11, 31, 10, 30))
  })

  it('不修改入参对象', () => {
    const input = new Date(2026, 6, 26)
    addDays(input, 5)
    expect(input.getDate()).toBe(26)
  })
})
