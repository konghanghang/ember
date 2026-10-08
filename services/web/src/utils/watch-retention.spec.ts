import { describe, expect, it } from 'vitest'
import { watchRetentionText, validateWatchRetention } from './watch-retention'
describe('观看要求提示', () => {
 it('关闭保号完全不展示', () => expect(watchRetentionText(undefined, 'Asia/Shanghai')).toBe(''))
 it('读取配置且不向用户暴露观察期术语', () => {
  const text=watchRetentionText({days:14,minMinutes:90,state:'waiting'},'Asia/Shanghai')
  expect(text).toBe('观看要求：生效满 14 天后，每天检查最近 14 天是否累计观看至少 90 分钟。')
  expect(text).not.toContain('观察期')
 })
 it('区分首次检查与每日检查', () => {
  expect(watchRetentionText({days:30,minMinutes:60,state:'checking'},'Asia/Shanghai')).toBe('观看要求：最近 30 天累计观看至少 60 分钟，每天检查。')
  expect(watchRetentionText({days:30,minMinutes:60,state:'grace',firstCheckAt:'2026-11-07T18:00:00Z'},'Asia/Shanghai')).toContain('11月8日')
 })
 it('失效权益不暗示会自动恢复', () => expect(watchRetentionText({days:30,minMinutes:60,state:'invalidated'},'Asia/Shanghai')).toBe('观看要求：最近 30 天累计观看至少 60 分钟。'))
})

describe('保号配置校验', () => {
 it('默认关闭可留空门槛，启用时不能为零或小数', () => {
  expect(validateWatchRetention({watchRetentionEnabled:false,watchRetentionDays:30,watchRetentionMinMinutes:0})).toBe('')
  for (const n of [0,-1,1.5,5256001]) expect(validateWatchRetention({watchRetentionEnabled:true,watchRetentionDays:30,watchRetentionMinMinutes:n})).not.toBe('')
  expect(validateWatchRetention({watchRetentionEnabled:true,watchRetentionDays:14,watchRetentionMinMinutes:60})).toBe('')
 })
})
