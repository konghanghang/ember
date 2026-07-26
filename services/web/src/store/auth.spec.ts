/// <reference types="node" />
// 批次 4 特征测试：锁住 store/auth.ts 在重构前的当前行为。
// 这些测试在 P2-5/P2-6/P3 重构后必须仍然全绿，仅在「行为变化点」上更新断言。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const loginMock = vi.fn()
const logoutMock = vi.fn()
const registerMock = vi.fn()
const getLoginProtectionConfigMock = vi.fn()

vi.mock('@/api/auth', () => ({
  login: (...args: unknown[]) => loginMock(...args),
  logout: (...args: unknown[]) => logoutMock(...args),
  register: (...args: unknown[]) => registerMock(...args),
  getLoginProtectionConfig: (...args: unknown[]) => getLoginProtectionConfigMock(...args),
}))

import { useAuthStore } from './auth'
import { useUserStore } from './user'
import { useConsoleStore } from './console'
import { resetAllStores } from './reset'
import type { LoginResponse, RegisterResponse, UserInfo } from '@/types/api'

const AUTH_TOKEN_KEY = 'token'
const LEGACY_ROLE_KEY = 'role'
const LEGACY_PASSWORD_KEY = 'passwordResetRequired'

function buildUser(overrides: Partial<UserInfo> = {}): UserInfo {
  return {
    id: 'u1',
    username: 'tester',
    role: 'user',
    ...overrides,
  } as UserInfo
}

function buildLoginResponse(overrides: Partial<LoginResponse> = {}): LoginResponse {
  return {
    token: 'tok-abc',
    user: buildUser({ role: 'admin', passwordResetRequired: false }),
    ...overrides,
  }
}

function dispatchStorageEvent(payload: {
  key: string | null
  newValue: string | null
  oldValue?: string | null
}) {
  const event = new StorageEvent('storage', {
    key: payload.key,
    newValue: payload.newValue,
    oldValue: payload.oldValue ?? null,
    storageArea: window.localStorage,
  })
  window.dispatchEvent(event)
}

describe('auth store — current behavior characterization', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.localStorage.clear()
    loginMock.mockReset()
    logoutMock.mockReset()
    registerMock.mockReset()
    getLoginProtectionConfigMock.mockReset()
  })

  describe('login state flow', () => {
    it('clears console+user stores before login and writes token+role after login', async () => {
      loginMock.mockResolvedValue(buildLoginResponse())
      const auth = useAuthStore()
      const user = useUserStore()
      const consoleStore = useConsoleStore()
      const userSpy = vi.spyOn(user, 'clearUserData')
      const consoleSpy = vi.spyOn(consoleStore, 'clearConsoleData')

      await auth.login({ username: 'a', password: 'b' })

      // 顺序：先清场（before request），再写 token+role
      expect(loginMock).toHaveBeenCalledWith({ username: 'a', password: 'b' })
      expect(userSpy).toHaveBeenCalled()
      expect(consoleSpy).toHaveBeenCalled()
      expect(auth.token).toBe('tok-abc')
      expect(auth.role).toBe('admin')
      expect(auth.passwordResetRequired).toBe(false)
      expect(window.localStorage.getItem(AUTH_TOKEN_KEY)).toBe('tok-abc')
    })

    it('tears down session on logout regardless of API success', async () => {
      logoutMock.mockRejectedValue(new Error('network'))
      const auth = useAuthStore()
      const user = useUserStore()
      auth.setAuth('tok-xyz', buildUser({ role: 'admin' }))
      user.setProfile(buildUser({ role: 'admin' }))

      await expect(auth.logout()).rejects.toThrow('network')

      expect(auth.token).toBeNull()
      expect(auth.role).toBeNull()
      expect(auth.passwordResetRequired).toBe(false)
      expect(user.profile).toBeNull()
      expect(window.localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
    })

    it('setAuth writes token, role and passwordResetRequired, clears legacy keys', () => {
      const auth = useAuthStore()
      window.localStorage.setItem(LEGACY_ROLE_KEY, 'admin')
      window.localStorage.setItem(LEGACY_PASSWORD_KEY, 'true')

      auth.setAuth('tok-1', buildUser({ role: 'user', passwordResetRequired: true }))

      expect(auth.token).toBe('tok-1')
      expect(auth.role).toBe('user')
      expect(auth.passwordResetRequired).toBe(true)
      expect(window.localStorage.getItem(AUTH_TOKEN_KEY)).toBe('tok-1')
      expect(window.localStorage.getItem(LEGACY_ROLE_KEY)).toBeNull()
      expect(window.localStorage.getItem(LEGACY_PASSWORD_KEY)).toBeNull()
    })

    it('clearAuth clears token + storage (role/passwordResetRequired now derived from profile, not managed here)', () => {
      // P2-5 后 role/passwordResetRequired 从 user store.profile 派生；clearAuth 只负责 token + localStorage。
      // 完整 teardown（含 profile/console）由 resetAllStores 统一处理。
      const auth = useAuthStore()
      auth.setAuth('tok-2', buildUser({ role: 'admin', passwordResetRequired: true }))

      auth.clearAuth()

      expect(auth.token).toBeNull()
      expect(window.localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
    })

    it('setPasswordResetRequired flips the flag in place', () => {
      const auth = useAuthStore()
      auth.setAuth('t', buildUser({ role: 'user', passwordResetRequired: false }))

      auth.setPasswordResetRequired(true)
      expect(auth.passwordResetRequired).toBe(true)

      auth.setPasswordResetRequired(false)
      expect(auth.passwordResetRequired).toBe(false)
    })
  })

  describe('restoreAuth — current behavior', () => {
    it('reads token from localStorage into memory', () => {
      window.localStorage.setItem(AUTH_TOKEN_KEY, 'tok-restored')
      const auth = useAuthStore()

      auth.restoreAuth()

      expect(auth.token).toBe('tok-restored')
      // 旧实现：恢复时 role/passwordResetRequired 不会被显式设置，但新值通过 setAuth 才会更新
      // 这里仅锁定 token 行为
      expect(auth.isAuthenticated).toBe(true)
    })

    it('keeps in-memory role when restoring the same token (no reset)', () => {
      window.localStorage.setItem(AUTH_TOKEN_KEY, 'same-tok')
      const auth = useAuthStore()
      auth.setAuth('same-tok', buildUser({ role: 'admin' }))

      auth.restoreAuth()

      expect(auth.role).toBe('admin')
    })

    it('locks the GHOST LOGIN fix: stale in-memory state is cleared when no token in storage', () => {
      // 重构后（P3 修复）：storage 无 token 时 restoreAuth 强制清场，杜绝幽灵登录。
      const auth = useAuthStore()
      const user = useUserStore()
      auth.setAuth('stale-tok', buildUser({ role: 'admin' }))
      user.setProfile(buildUser({ role: 'admin' }))
      // 模拟「内存有，存储没有」（如其他 tab 调用了 localStorage.removeItem 但本 tab 不知情）
      window.localStorage.removeItem(AUTH_TOKEN_KEY)

      auth.restoreAuth()

      expect(auth.token).toBeNull()
      expect(auth.role).toBeNull()
      expect(user.profile).toBeNull()
    })

    it('clears legacy storage keys on restore', () => {
      window.localStorage.setItem(LEGACY_ROLE_KEY, 'admin')
      window.localStorage.setItem(LEGACY_PASSWORD_KEY, 'true')
      const auth = useAuthStore()

      auth.restoreAuth()

      expect(window.localStorage.getItem(LEGACY_ROLE_KEY)).toBeNull()
      expect(window.localStorage.getItem(LEGACY_PASSWORD_KEY)).toBeNull()
    })

    it('preserves in-memory session when localStorage is unavailable (sandbox/privacy mode)', () => {
      // localStorage 抛 SecurityError 时，login/register 已退化为内存会话；
      // restoreAuth 不能把「存储不可用」误判为「无 token」而清掉刚登录的内存会话。
      const auth = useAuthStore()
      const user = useUserStore()
      auth.setAuth('mem-tok', buildUser({ role: 'admin' }))
      user.setProfile(buildUser({ role: 'admin' }))

      const getItemSpy = vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
        throw new DOMException('blocked', 'SecurityError')
      })
      const setItemSpy = vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
        throw new DOMException('blocked', 'SecurityError')
      })
      const removeItemSpy = vi.spyOn(window.localStorage, 'removeItem').mockImplementation(() => {
        throw new DOMException('blocked', 'SecurityError')
      })

      auth.restoreAuth()

      expect(auth.token).toBe('mem-tok')
      expect(auth.role).toBe('admin')
      expect(user.profile).not.toBeNull()

      getItemSpy.mockRestore()
      setItemSpy.mockRestore()
      removeItemSpy.mockRestore()
    })
  })

  describe('cross-tab sync — current behavior', () => {
    it('syncs sign-out when another tab calls localStorage.clear() (key===null)', () => {
      // clear() 删除了 token，本 tab 必须随之登出：重新读取发现 token 已无，
      // 清掉内存 token + profile 并通知 'signed-out'。早期实现误把 clear() 当噪音忽略，导致失效会话残留。
      const auth = useAuthStore()
      const user = useUserStore()
      auth.setAuth('tab-tok', buildUser({ role: 'admin' }))
      user.setProfile(buildUser({ role: 'admin' }))
      // 模拟另一 tab clear() 后 storage 的实际状态：token 键已不存在。
      window.localStorage.removeItem(AUTH_TOKEN_KEY)
      let captured: string | null = null
      auth.initCrossTabSync((reason) => {
        captured = reason
      })

      dispatchStorageEvent({ key: null, newValue: null })

      expect(auth.token).toBeNull()
      expect(auth.role).toBeNull()
      expect(user.profile).toBeNull()
      expect(captured).toBe('signed-out')
    })

    it('ignores storage events for unrelated keys', () => {
      const auth = useAuthStore()
      auth.setAuth('tab-tok-2', buildUser({ role: 'admin' }))
      let captured: string | null = null
      auth.initCrossTabSync((reason) => {
        captured = reason
      })

      dispatchStorageEvent({ key: 'unrelated-key', newValue: 'x' })

      expect(auth.token).toBe('tab-tok-2')
      expect(auth.role).toBe('admin')
      expect(captured).toBeNull()
    })

    it('handles AUTH_TOKEN_KEY storage event (token updated in another tab)', () => {
      const auth = useAuthStore()
      const user = useUserStore()
      auth.setAuth('old-tok', buildUser({ role: 'admin' }))
      user.setProfile(buildUser({ role: 'admin' }))
      let captured: string | null = null
      auth.initCrossTabSync((reason) => {
        captured = reason
      })

      window.localStorage.setItem(AUTH_TOKEN_KEY, 'new-tok')
      dispatchStorageEvent({ key: AUTH_TOKEN_KEY, newValue: 'new-tok', oldValue: 'old-tok' })

      expect(auth.token).toBe('new-tok')
      expect(captured).toBe('updated')
    })

    it('destroyCrossTabSync stops future storage events', () => {
      const auth = useAuthStore()
      auth.setAuth('t1', buildUser({ role: 'admin' }))
      auth.initCrossTabSync(() => {})

      auth.destroyCrossTabSync()

      window.localStorage.removeItem(AUTH_TOKEN_KEY)
      dispatchStorageEvent({ key: AUTH_TOKEN_KEY, newValue: null })
      // 已经移除监听，store 不再被同步
      expect(auth.crossTabSyncEnabled).toBe(false)
    })
  })

  describe('loadProtectionConfig — TTL + explicit invalidation (P3 fix)', () => {
    it('caches protectionConfig within TTL window', async () => {
      getLoginProtectionConfigMock.mockResolvedValue({
        turnstileLoginEnabled: true,
        turnstileSiteKey: 'first-key',
      })

      const auth = useAuthStore()
      await auth.loadProtectionConfig()
      expect(getLoginProtectionConfigMock).toHaveBeenCalledTimes(1)

      // TTL 内重复调用：命中缓存
      await auth.loadProtectionConfig()
      expect(getLoginProtectionConfigMock).toHaveBeenCalledTimes(1)
      expect(auth.protectionConfig?.turnstileSiteKey).toBe('first-key')
    })

    it('force=true bypasses cache even within TTL', async () => {
      getLoginProtectionConfigMock.mockResolvedValueOnce({
        turnstileLoginEnabled: true,
        turnstileSiteKey: 'first',
      })
      getLoginProtectionConfigMock.mockResolvedValueOnce({
        turnstileLoginEnabled: false,
        turnstileSiteKey: 'second',
      })

      const auth = useAuthStore()
      await auth.loadProtectionConfig()
      const second = await auth.loadProtectionConfig({ force: true })

      expect(second.turnstileSiteKey).toBe('second')
      expect(getLoginProtectionConfigMock).toHaveBeenCalledTimes(2)
    })

    it('invalidateProtectionConfig forces next load to refetch', async () => {
      getLoginProtectionConfigMock.mockResolvedValue({
        turnstileLoginEnabled: true,
        turnstileSiteKey: 'cached',
      })

      const auth = useAuthStore()
      await auth.loadProtectionConfig()
      expect(getLoginProtectionConfigMock).toHaveBeenCalledTimes(1)

      auth.invalidateProtectionConfig()
      expect(auth.protectionConfig).toBeNull()

      getLoginProtectionConfigMock.mockResolvedValue({
        turnstileLoginEnabled: true,
        turnstileSiteKey: 'refreshed',
      })
      const next = await auth.loadProtectionConfig()
      expect(next.turnstileSiteKey).toBe('refreshed')
      expect(getLoginProtectionConfigMock).toHaveBeenCalledTimes(2)
    })
  })

  describe('resetAllStores — unified teardown (P3 fix)', () => {
    it('clears auth+user+console stores in one call', () => {
      const auth = useAuthStore()
      const user = useUserStore()
      const consoleStore = useConsoleStore()
      auth.setAuth('full-tok', buildUser({ role: 'admin', passwordResetRequired: true }))
      user.setProfile(buildUser({ role: 'admin', passwordResetRequired: true }))

      resetAllStores()

      expect(auth.token).toBeNull()
      expect(auth.role).toBeNull()
      expect(auth.passwordResetRequired).toBe(false)
      expect(user.profile).toBeNull()
      expect(window.localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
      // consoleStore 状态空（accountLinks 数组）
      expect(consoleStore.accountLinks).toEqual([])
    })
  })

  describe('register state flow', () => {
    it('writes new token+role after a successful register', async () => {
      const response: RegisterResponse = {
        token: 'reg-tok',
        user: buildUser({ role: 'user', passwordResetRequired: true }),
      }
      registerMock.mockResolvedValue(response)
      const auth = useAuthStore()
      const user = useUserStore()

      await auth.register({
        username: 'new',
        password: 'pw',
        email: 'a@b.com',
        code: '1234',
      })

      expect(auth.token).toBe('reg-tok')
      expect(auth.role).toBe('user')
      expect(auth.passwordResetRequired).toBe(true)
      expect(user.profile?.id).toBe('u1')
      expect(window.localStorage.getItem(AUTH_TOKEN_KEY)).toBe('reg-tok')
    })

    it('preserves the existing session when register fails (does not clear token before request)', async () => {
      // 已登录用户从未受保护的 /register 提交一个会失败的注册请求时，不能被意外登出：
      // 会话切换只能在注册成功后发生。
      registerMock.mockRejectedValue(new Error('register failed'))
      const auth = useAuthStore()
      const user = useUserStore()
      auth.setAuth('existing-tok', buildUser({ role: 'admin' }))
      user.setProfile(buildUser({ role: 'admin' }))

      await expect(
        auth.register({
          username: 'new',
          password: 'pw',
          email: 'a@b.com',
          code: '1234',
        })
      ).rejects.toThrow('register failed')

      expect(auth.token).toBe('existing-tok')
      expect(auth.role).toBe('admin')
      expect(user.profile?.role).toBe('admin')
    })
  })
})
