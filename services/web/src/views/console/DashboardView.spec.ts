import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DashboardView from './DashboardView.vue'
import { getMediaStats } from '@/api/console'

vi.mock('@/api/console', () => ({
  getMediaStats: vi.fn(),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push: vi.fn() }),
  }
})

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const authStoreState = reactive({
  isAdmin: false,
})

const userStoreState = reactive({
  profile: {
    id: 'user_1',
    username: 'demo',
    role: 'user' as 'user' | 'admin',
    email: 'demo@example.com',
    embyId: 'emby_1',
    expiresAt: '2026-01-01T00:00:00Z',
    embyDisabled: true,
    isActive: true,
    createdAt: '2025-01-01T00:00:00Z',
  },
  embyUrl: 'https://emby.example.com',
  embyConfigured: true,
})

const fetchEmbyConfigMock = vi.fn()
const clearEmbyUrlMock = vi.fn()
const setEmbyConfiguredMock = vi.fn()

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(() => ({
    get isAdmin() {
      return authStoreState.isAdmin
    },
  })),
}))

vi.mock('@/store/user', () => ({
  useUserStore: vi.fn(() => ({
    get profile() {
      return userStoreState.profile
    },
    get embyUrl() {
      return userStoreState.embyUrl
    },
    get embyConfigured() {
      return userStoreState.embyConfigured
    },
    fetchEmbyConfig: fetchEmbyConfigMock,
    clearEmbyUrl: clearEmbyUrlMock,
    setEmbyConfigured: setEmbyConfiguredMock,
  })),
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const EmberEmptyStateCardStub = defineComponent({
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  setup(props, { slots }) {
    return () =>
      h('div', { 'data-test': 'empty-state-card' }, [
        h('div', props.title),
        h('div', props.description),
        slots.actions?.(),
      ])
  },
})

function mountView() {
  return mount(DashboardView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        'el-icon': passthroughStub,
        RecentLibrarySection: defineComponent({
          setup() {
            return () => h('div', { 'data-test': 'recent-library' }, 'recent-library')
          },
        }),
        EmberEmptyStateCard: EmberEmptyStateCardStub,
        CircleCloseFilled: passthroughStub,
        CopyDocument: passthroughStub,
        Film: passthroughStub,
        Monitor: passthroughStub,
        VideoPlay: passthroughStub,
      },
    },
  })
}

describe('DashboardView 过期用户概览', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    authStoreState.isAdmin = false
    userStoreState.profile.expiresAt = '2026-01-01T00:00:00Z'
    userStoreState.profile.embyDisabled = true
    userStoreState.embyUrl = 'https://emby.example.com'
    userStoreState.embyConfigured = true

    fetchEmbyConfigMock.mockResolvedValue({
      configured: true,
      url: 'https://emby.example.com',
    })
    vi.mocked(getMediaStats).mockResolvedValue({
      success: true,
      configured: true,
      data: {
        movieCount: 321,
        seriesCount: 45,
        episodeCount: 6789,
      },
    })
  })

  it('过期用户仍请求并展示片库统计，同时保持 Emby 入口锁定', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(fetchEmbyConfigMock).toHaveBeenCalledTimes(1)
    expect(getMediaStats).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('321')
    expect(wrapper.text()).toContain('45')
    expect(wrapper.text()).toContain('6789')
    expect(wrapper.text()).toContain('服务器访问已锁定')
    expect(wrapper.text()).not.toContain('当前可用')
    expect(wrapper.text()).not.toContain('打开')
  })
})
