import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { routes, installAuthGuards } from './index'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'
import type { UserInfo } from '@/types/api'

const getProfileMock = vi.fn()
const warningMock = vi.fn()

vi.mock('@/api/console', () => ({
  getProfile: (...args: unknown[]) => getProfileMock(...args),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    warning: (...args: unknown[]) => warningMock(...args),
  },
}))

function buildUser(overrides: Partial<UserInfo> = {}): UserInfo {
  return {
    id: 'u1',
    username: 'tester',
    role: 'user',
    passwordResetRequired: false,
    ...overrides,
  } as UserInfo
}

function buildTestRouter() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes,
  })
  installAuthGuards(router)
  return router
}

describe('router auth guard', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.localStorage.clear()
    getProfileMock.mockReset()
    warningMock.mockReset()
  })

  it('loads profile and enters a protected route for a valid user session', async () => {
    getProfileMock.mockResolvedValue(buildUser())
    const authStore = useAuthStore()
    authStore.setAuth('tok-user')
    const router = buildTestRouter()

    await router.push('/console/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('console-dashboard')
    expect(getProfileMock).toHaveBeenCalledTimes(1)
    expect(authStore.token).toBe('tok-user')
    expect(useUserStore().profile?.role).toBe('user')
  })

  it('clears identity and redirects to login when profile loading returns 401', async () => {
    getProfileMock.mockRejectedValue({ response: { status: 401 } })
    const authStore = useAuthStore()
    authStore.setAuth('expired-token')
    const router = buildTestRouter()

    await router.push('/console/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query).toEqual({ redirect: '/console/dashboard' })
    expect(authStore.token).toBeNull()
    expect(useUserStore().profile).toBeNull()
  })

  it('keeps token and opens login recovery mode when profile loading hits a transient failure', async () => {
    getProfileMock.mockRejectedValue({ response: { status: 503 } })
    const authStore = useAuthStore()
    authStore.setAuth('recoverable-token')
    const router = buildTestRouter()

    await router.push('/console/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query).toEqual({
      redirect: '/console/dashboard',
      recovery: 'profile',
    })
    expect(authStore.token).toBe('recoverable-token')
    expect(useUserStore().profile).toBeNull()
  })

  it('allows login recovery route to render when a token exists but profile is still unavailable', async () => {
    getProfileMock.mockRejectedValue(new Error('network down'))
    const authStore = useAuthStore()
    authStore.setAuth('recoverable-token')
    const router = buildTestRouter()

    await router.push('/login?redirect=/console/dashboard&recovery=profile')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
    expect(getProfileMock).not.toHaveBeenCalled()
    expect(authStore.token).toBe('recoverable-token')
  })

  it('treats recovery query as display-only and redirects when profile already exists', async () => {
    const authStore = useAuthStore()
    const profile = buildUser()
    authStore.setAuth('tok-ready', profile)
    useUserStore().setProfile(profile)
    const router = buildTestRouter()

    await router.push('/login?redirect=/console/dashboard&recovery=profile')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('console-dashboard')
    expect(getProfileMock).not.toHaveBeenCalled()
  })

  it('redirects authenticated users away from login outside recovery mode', async () => {
    const authStore = useAuthStore()
    authStore.setAuth('tok-ready', buildUser())
    const router = buildTestRouter()

    await router.push('/login')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('console-dashboard')
    expect(getProfileMock).not.toHaveBeenCalled()
  })

  it('blocks admin routes for a regular user after profile loading', async () => {
    getProfileMock.mockResolvedValue(buildUser({ role: 'user' }))
    const authStore = useAuthStore()
    authStore.setAuth('tok-user')
    const router = buildTestRouter()

    await router.push('/console/users')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('console-dashboard')
    expect(warningMock).toHaveBeenCalledWith('当前账号无权访问该页面')
  })

  it('sends forced-password-reset users to account center before protected content renders', async () => {
    getProfileMock.mockResolvedValue(buildUser({ passwordResetRequired: true }))
    const authStore = useAuthStore()
    authStore.setAuth('tok-reset')
    const router = buildTestRouter()

    await router.push('/console/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('console-account')
    expect(warningMock).toHaveBeenCalledWith('当前账号必须先修改密码')
  })
})

describe('console 115 account routes', () => {
  it('主路由受管理员角色保护，旧管理路径只做重定向', () => {
    const router = buildTestRouter()
    const mainRoute = router.getRoutes().find(route => route.name === 'console-p115-accounts')
    expect(mainRoute?.path).toBe('/console/p115-accounts')
    expect(mainRoute?.meta.role).toBe('admin')

    const legacyRoute = router.getRoutes().find(route => route.path === '/admin/p115-accounts')
    expect(legacyRoute?.redirect).toBe('/console/p115-accounts')
  })

  it('个人 115 网盘使用独立用户控制台路由', () => {
    const router = buildTestRouter()
    const personalRoute = router.getRoutes().find(route => route.name === 'console-p115')
    expect(personalRoute?.path).toBe('/console/p115')
    expect(personalRoute?.meta.role).toBe('user')
  })
})

describe('plan group page routes', () => {
  it.each(['/console/plan-groups', '/admin/plan-groups'])('%s 不再注册独立页面或兼容重定向', (path) => {
    const router = buildTestRouter()
    expect(router.getRoutes().find(route => route.path === path)).toBeUndefined()
  })
})
