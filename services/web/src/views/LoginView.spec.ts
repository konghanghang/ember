import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import LoginView from './LoginView.vue'
import { installAuthGuards, routes } from '@/router'
import { useAuthStore } from '@/store/auth'
import type { UserInfo } from '@/types/api'

const getProfileMock = vi.fn()
const getLoginProtectionConfigMock = vi.fn()
const warningMock = vi.fn()
const errorMock = vi.fn()
const successMock = vi.fn()

vi.mock('@/api/console', () => ({
  getProfile: (...args: unknown[]) => getProfileMock(...args),
}))

vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  register: vi.fn(),
  getLoginProtectionConfig: (...args: unknown[]) => getLoginProtectionConfigMock(...args),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    warning: (...args: unknown[]) => warningMock(...args),
    error: (...args: unknown[]) => errorMock(...args),
    success: (...args: unknown[]) => successMock(...args),
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

async function mountLoginAt(path: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes,
  })
  installAuthGuards(router)
  useAuthStore().setAuth('recoverable-token')
  await router.push(path)
  await router.isReady()

  const wrapper = mount(LoginView, {
    global: {
      plugins: [pinia, router],
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        ElIcon: { template: '<span><slot /></span>' },
        ElForm: { template: '<form><slot /></form>' },
        ElInput: { template: '<input />' },
        ElButton: {
          props: ['disabled', 'loading'],
          template: '<button type="button" :disabled="disabled || loading"><slot /></button>',
        },
        TurnstileWidget: true,
      },
    },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('LoginView session recovery', () => {
  beforeEach(() => {
    window.localStorage.clear()
    getProfileMock.mockReset()
    getLoginProtectionConfigMock.mockResolvedValue({
      turnstileLoginEnabled: false,
      turnstileSiteKey: '',
    })
    warningMock.mockReset()
    errorMock.mockReset()
    successMock.mockReset()
  })

  it('retries profile loading from the existing login page recovery state', async () => {
    getProfileMock.mockResolvedValue(buildUser())
    const { wrapper, router } = await mountLoginAt('/login?redirect=/console/dashboard&recovery=profile')

    expect(wrapper.text()).toContain('会话暂时无法恢复')
    expect(wrapper.find('[role="status"][aria-live="polite"]').exists()).toBe(true)

    const retryButton = wrapper.findAll('button').find((button) => button.text().includes('重试'))
    expect(retryButton, '未找到恢复重试按钮').toBeTruthy()
    await retryButton!.trigger('click')
    await flushPromises()

    expect(getProfileMock).toHaveBeenCalledTimes(1)
    await vi.waitFor(() => {
      expect(router.currentRoute.value.name).toBe('console-dashboard')
    })
  })

  it('keeps the token and recovery page when retrying profile loading still fails', async () => {
    getProfileMock.mockRejectedValue({ response: { status: 503 } })
    const { wrapper, router } = await mountLoginAt('/login?redirect=/console/dashboard&recovery=profile')

    const retryButton = wrapper.findAll('button').find((button) => button.text().includes('重试'))
    expect(retryButton, '未找到恢复重试按钮').toBeTruthy()
    await retryButton!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('login')
    expect(useAuthStore().token).toBe('recoverable-token')
    expect(wrapper.text()).toContain('会话暂时无法恢复')
    expect(warningMock).not.toHaveBeenCalled()
  })

  it('lets the guard send a recovered forced-password-reset user to account center', async () => {
    getProfileMock.mockResolvedValue(buildUser({ passwordResetRequired: true }))
    const { wrapper, router } = await mountLoginAt('/login?redirect=/console/dashboard&recovery=profile')

    const retryButton = wrapper.findAll('button').find((button) => button.text().includes('重试'))
    expect(retryButton, '未找到恢复重试按钮').toBeTruthy()
    await retryButton!.trigger('click')
    await flushPromises()

    await vi.waitFor(() => {
      expect(router.currentRoute.value.name).toBe('console-account')
    })
    expect(warningMock).toHaveBeenCalledWith('当前账号必须先修改密码')
  })

  it('lets the guard enforce role checks after recovery succeeds', async () => {
    getProfileMock.mockResolvedValue(buildUser({ role: 'user' }))
    const { wrapper, router } = await mountLoginAt('/login?redirect=/console/users&recovery=profile')

    const retryButton = wrapper.findAll('button').find((button) => button.text().includes('重试'))
    expect(retryButton, '未找到恢复重试按钮').toBeTruthy()
    await retryButton!.trigger('click')
    await flushPromises()

    await vi.waitFor(() => {
      expect(router.currentRoute.value.name).toBe('console-dashboard')
    })
    expect(warningMock).toHaveBeenCalledWith('当前账号无权访问该页面')
  })

  it('clears recovery state on retry 401 without adding a local warning toast', async () => {
    getProfileMock.mockRejectedValue({ response: { status: 401 } })
    const { wrapper, router } = await mountLoginAt('/login?redirect=/console/dashboard&recovery=profile')

    const retryButton = wrapper.findAll('button').find((button) => button.text().includes('重试'))
    expect(retryButton, '未找到恢复重试按钮').toBeTruthy()
    await retryButton!.trigger('click')
    await flushPromises()

    await vi.waitFor(() => {
      expect(router.currentRoute.value.name).toBe('login')
      expect(router.currentRoute.value.query).toEqual({ redirect: '/console/dashboard' })
    })
    expect(useAuthStore().token).toBeNull()
    expect(warningMock).not.toHaveBeenCalled()
  })
})
