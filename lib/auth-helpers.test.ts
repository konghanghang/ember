import { describe, it, expect } from 'vitest'
import {
  validatePasswordStrength,
  isPasswordComplex,
  hashPassword,
} from './auth-helpers'

describe('auth-helpers', () => {
  describe('validatePasswordStrength', () => {
    it('应该接受有效密码（6个字符）', () => {
      const result = validatePasswordStrength('abcdef')

      expect(result.valid).toBe(true)
      expect(result.reason).toBeUndefined()
    })

    it('应该接受有效密码（更长）', () => {
      const result = validatePasswordStrength('veryStrongPassword123!')

      expect(result.valid).toBe(true)
    })

    it('应该拒绝太短的密码（5个字符）', () => {
      const result = validatePasswordStrength('abcde')

      expect(result.valid).toBe(false)
      expect(result.reason).toBe('密码长度至少 6 个字符')
    })

    it('应该拒绝空密码', () => {
      const result = validatePasswordStrength('')

      expect(result.valid).toBe(false)
      expect(result.reason).toBe('密码长度至少 6 个字符')
    })

    it('应该接受恰好 6 个字符的密码', () => {
      const result = validatePasswordStrength('123456')

      expect(result.valid).toBe(true)
    })
  })

  describe('isPasswordComplex', () => {
    it('应该判断密码符合复杂度要求', () => {
      expect(isPasswordComplex('abcdef')).toBe(true)
    })

    it('应该判断密码不符合复杂度要求', () => {
      expect(isPasswordComplex('abc')).toBe(false)
    })

    it('应该判断恰好 6 个字符的密码', () => {
      expect(isPasswordComplex('123456')).toBe(true)
    })
  })

  describe('hashPassword', () => {
    it('应该成功加密密码', async () => {
      const password = 'testPassword123'

      const hashed = await hashPassword(password)

      expect(hashed).toBeTruthy()
      expect(hashed).not.toBe(password)
      expect(hashed.length).toBeGreaterThan(50) // bcrypt hash 长度约 60
    })

    it('应该使用指定的 rounds', async () => {
      const password = 'testPassword123'

      const hashed = await hashPassword(password, 4)

      expect(hashed).toBeTruthy()
      expect(hashed.startsWith('$2a$04$') || hashed.startsWith('$2b$04$')).toBe(true)
    })

    it('应该使用默认的 rounds（10）', async () => {
      const password = 'testPassword123'

      const hashed = await hashPassword(password)

      expect(hashed).toBeTruthy()
      expect(hashed.startsWith('$2a$10$') || hashed.startsWith('$2b$10$')).toBe(true)
    })

    it('应该为相同密码生成不同的哈希（salt）', async () => {
      const password = 'testPassword123'

      const hash1 = await hashPassword(password)
      const hash2 = await hashPassword(password)

      expect(hash1).not.toBe(hash2) // 不同的 salt
    })

    it('应该能验证加密后的密码', async () => {
      const bcrypt = await import('bcryptjs')
      const password = 'testPassword123'

      const hashed = await hashPassword(password)
      const isValid = await bcrypt.compare(password, hashed)

      expect(isValid).toBe(true)
    })
  })
})
