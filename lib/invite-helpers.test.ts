import { describe, it, expect } from 'vitest'
import {
  isInviteExpired,
  isInviteUsedUp,
  canInviteBeUsed,
  canInviteBeDeleted,
} from './invite-helpers'

describe('invite-helpers', () => {
  describe('isInviteExpired', () => {
    it('应该判断邀请码已过期', () => {
      const invite = {
        expiresAt: new Date('2020-01-01'),
      }

      expect(isInviteExpired(invite)).toBe(true)
    })

    it('应该判断邀请码未过期', () => {
      const invite = {
        expiresAt: new Date('2030-01-01'),
      }

      expect(isInviteExpired(invite)).toBe(false)
    })

    it('应该处理 null 到期时间（永不过期）', () => {
      const invite = {
        expiresAt: null,
      }

      expect(isInviteExpired(invite)).toBe(false)
    })
  })

  describe('isInviteUsedUp', () => {
    it('应该判断邀请码已用尽', () => {
      const invite = {
        usedCount: 5,
        maxUses: 5,
      }

      expect(isInviteUsedUp(invite)).toBe(true)
    })

    it('应该判断邀请码超额使用', () => {
      const invite = {
        usedCount: 6,
        maxUses: 5,
      }

      expect(isInviteUsedUp(invite)).toBe(true)
    })

    it('应该判断邀请码还有剩余次数', () => {
      const invite = {
        usedCount: 3,
        maxUses: 5,
      }

      expect(isInviteUsedUp(invite)).toBe(false)
    })

    it('应该判断未使用的邀请码', () => {
      const invite = {
        usedCount: 0,
        maxUses: 1,
      }

      expect(isInviteUsedUp(invite)).toBe(false)
    })
  })

  describe('canInviteBeUsed', () => {
    it('应该允许使用有效的邀请码', () => {
      const invite = {
        usedCount: 0,
        maxUses: 1,
        expiresAt: new Date('2030-01-01'),
      }

      const result = canInviteBeUsed(invite)

      expect(result.valid).toBe(true)
      expect(result.reason).toBeUndefined()
    })

    it('应该拒绝已用尽的邀请码', () => {
      const invite = {
        usedCount: 1,
        maxUses: 1,
        expiresAt: new Date('2030-01-01'),
      }

      const result = canInviteBeUsed(invite)

      expect(result.valid).toBe(false)
      expect(result.reason).toBe('邀请码已达使用上限')
    })

    it('应该拒绝已过期的邀请码', () => {
      const invite = {
        usedCount: 0,
        maxUses: 1,
        expiresAt: new Date('2020-01-01'),
      }

      const result = canInviteBeUsed(invite)

      expect(result.valid).toBe(false)
      expect(result.reason).toBe('邀请码已过期')
    })

    it('应该优先检查使用次数（即使已过期）', () => {
      const invite = {
        usedCount: 5,
        maxUses: 5,
        expiresAt: new Date('2020-01-01'),
      }

      const result = canInviteBeUsed(invite)

      expect(result.valid).toBe(false)
      expect(result.reason).toBe('邀请码已达使用上限')
    })

    it('应该允许无过期时间的邀请码', () => {
      const invite = {
        usedCount: 0,
        maxUses: 1,
        expiresAt: null,
      }

      const result = canInviteBeUsed(invite)

      expect(result.valid).toBe(true)
    })
  })

  describe('canInviteBeDeleted', () => {
    it('应该允许删除未使用的邀请码', () => {
      const invite = {
        usedCount: 0,
      }

      expect(canInviteBeDeleted(invite)).toBe(true)
    })

    it('应该拒绝删除已使用的邀请码', () => {
      const invite = {
        usedCount: 1,
      }

      expect(canInviteBeDeleted(invite)).toBe(false)
    })

    it('应该拒绝删除部分使用的邀请码', () => {
      const invite = {
        usedCount: 3,
      }

      expect(canInviteBeDeleted(invite)).toBe(false)
    })
  })
})
