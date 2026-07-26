/// <reference types="node" />
// 批次 4 特征测试：锁住 store/user.ts updatePassword 当前「双份存储」行为。
// P2-5 重构后该测试断言会更新（只写单一事实源）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const updatePasswordApiMock = vi.fn()
const getProfileMock = vi.fn()
const updateEmailApiMock = vi.fn()
const getMediaStatsMock = vi.fn()
const getEmbyConfigMock = vi.fn()

vi.mock('@/api/console', () => ({
  updatePassword: (...args: unknown[]) => updatePasswordApiMock(...args),
  getProfile: (...args: unknown[]) => getProfileMock(...args),
  updateEmail: (...args: unknown[]) => updateEmailApiMock(...args),
  getMediaStats: (...args: unknown[]) => getMediaStatsMock(...args),
  getEmbyConfig: (...args: unknown[]) => getEmbyConfigMock(...args),
  getConsoleAccountLinks: vi.fn(async () => ({ data: [] })),
}))

import { useAuthStore } from './auth'
import { useUserStore } from './user'
import type { UserInfo } from '@/types/api'

function buildUser(overrides: Partial<UserInfo> = {}): UserInfo {
  return {
    id: 'u1',
    username: 'tester',
    role: 'user',
    passwordResetRequired: true,
    ...overrides,
  } as UserInfo
}

describe('user store — current behavior characterization', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    updatePasswordApiMock.mockReset()
    getProfileMock.mockReset()
  })

  it('updatePassword writes passwordResetRequired=false once via single source of truth (P2-5)', async () => {
    // P2-5 后：只写 user store.profile 这一事实源，auth store.passwordResetRequired 通过 computed 派生。
    updatePasswordApiMock.mockResolvedValue(undefined)
    const user = useUserStore()
    const auth = useAuthStore()
    auth.setAuth('t', buildUser({ role: 'user', passwordResetRequired: true }))
    user.setProfile(buildUser({ role: 'user', passwordResetRequired: true }))

    await user.updatePassword('old', 'new')

    expect(user.profile?.passwordResetRequired).toBe(false)
    expect(auth.passwordResetRequired).toBe(false)
  })

  it('fetchProfile populates profile, role derives automatically (P2-5)', async () => {
    getProfileMock.mockResolvedValue(buildUser({ role: 'admin' }))
    const user = useUserStore()
    const auth = useAuthStore()

    await user.fetchProfile()

    expect(user.profile?.role).toBe('admin')
    expect(auth.role).toBe('admin')
  })
})
