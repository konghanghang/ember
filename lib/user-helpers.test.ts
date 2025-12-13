import { describe, it, expect } from 'vitest'
import {
  calculateUserExpiry,
  getDaysUntilExpiry,
  isUserExpired,
  shouldReactivateUser,
  getUserTargetStatus,
} from './user-helpers'

describe('user-helpers', () => {
  describe('calculateUserExpiry', () => {
    it('应该正确计算到期时间', () => {
      const baseDate = new Date('2025-01-01')
      const result = calculateUserExpiry(baseDate, 30)

      expect(result).toEqual(new Date('2025-01-31'))
    })

    it('应该处理跨月的情况', () => {
      const baseDate = new Date('2025-01-15')
      const result = calculateUserExpiry(baseDate, 30)

      expect(result).toEqual(new Date('2025-02-14'))
    })

    it('应该处理跨年的情况', () => {
      const baseDate = new Date('2024-12-01')
      const result = calculateUserExpiry(baseDate, 60)

      expect(result).toEqual(new Date('2025-01-30'))
    })
  })

  describe('getDaysUntilExpiry', () => {
    it('应该返回正确的剩余天数', () => {
      const now = new Date()
      const futureDate = new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000)

      const days = getDaysUntilExpiry(futureDate)

      expect(days).toBe(30)
    })

    it('应该返回负数表示已过期', () => {
      const now = new Date()
      const pastDate = new Date(now.getTime() - 5 * 24 * 60 * 60 * 1000)

      const days = getDaysUntilExpiry(pastDate)

      expect(days).toBeLessThan(0)
    })

    it('应该处理 null 值', () => {
      const days = getDaysUntilExpiry(null)

      expect(days).toBe(-1)
    })
  })

  describe('isUserExpired', () => {
    it('应该判断用户已过期', () => {
      const user = {
        expiresAt: new Date('2020-01-01'),
      }

      expect(isUserExpired(user)).toBe(true)
    })

    it('应该判断用户未过期', () => {
      const user = {
        expiresAt: new Date('2030-01-01'),
      }

      expect(isUserExpired(user)).toBe(false)
    })

    it('应该处理 null 到期时间', () => {
      const user = {
        expiresAt: null,
      }

      expect(isUserExpired(user)).toBe(false)
    })
  })

  describe('shouldReactivateUser', () => {
    it('应该判断需要重新启用（用户被禁用且新到期时间在未来）', () => {
      const user = { isActive: false }
      const newExpiry = new Date('2030-01-01')

      expect(shouldReactivateUser(user, newExpiry)).toBe(true)
    })

    it('应该判断不需要重新启用（用户已激活）', () => {
      const user = { isActive: true }
      const newExpiry = new Date('2030-01-01')

      expect(shouldReactivateUser(user, newExpiry)).toBe(false)
    })

    it('应该判断不需要重新启用（新到期时间在过去）', () => {
      const user = { isActive: false }
      const newExpiry = new Date('2020-01-01')

      expect(shouldReactivateUser(user, newExpiry)).toBe(false)
    })
  })

  describe('getUserTargetStatus', () => {
    it('应该返回正确的目标状态（从激活到禁用）', () => {
      const user = { isActive: true }

      const result = getUserTargetStatus(user)

      expect(result).toEqual({
        isActive: false,
        isDisabled: true,
      })
    })

    it('应该返回正确的目标状态（从禁用到激活）', () => {
      const user = { isActive: false }

      const result = getUserTargetStatus(user)

      expect(result).toEqual({
        isActive: true,
        isDisabled: false,
      })
    })
  })
})
