import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import PlanGroupsView from '@/views/admin/PlanGroupsView.vue'
import AccountCenterView from '@/views/console/AccountCenterView.vue'
import RankingsView from '@/views/console/RankingsView.vue'
import SubscriptionsView from '@/views/console/SubscriptionsView.vue'
import request from '@/api/request'
import { login } from '@/api/auth'
import { getRankingLibraryAllowlist, updatePlanGroup } from '@/api/admin'
import { createSubscription } from '@/api/console'
import type { LoginResponse, UserInfo } from '@/types/api'
import { startGoIntegrationServer, type RunningGoServer } from './go-server'

const describeIntegration = process.env.EMBER_WEB_RUN_INTEGRATION === '1' ? describe : describe.skip

vi.mock('element-plus', () => ({
  ElMessage: {
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
  ElMessageBox: {
    confirm: vi.fn(() => Promise.resolve()),
  },
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => ({ query: {} }),
    useRouter: () => ({ push: vi.fn() }),
  }
})

const authState = reactive({
  token: null as string | null,
  isAdmin: false,
})

const userStoreState = reactive({
  profile: null as UserInfo | null,
})

const fetchProfileMock = vi.fn(async () => {})
const setProfileMock = vi.fn((profile: UserInfo | null) => {
  userStoreState.profile = profile
})

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(() => ({
    get token() {
      return authState.token
    },
    get isAdmin() {
      return authState.isAdmin
    },
    clearAuth: vi.fn(),
  })),
}))

vi.mock('@/store/user', () => ({
  useUserStore: vi.fn(() => ({
    get profile() {
      return userStoreState.profile
    },
    setProfile: setProfileMock,
    fetchProfile: fetchProfileMock,
    clearUserData: vi.fn(),
    updateEmail: vi.fn(),
  })),
}))

vi.mock('@/store/console', () => ({
  useConsoleStore: vi.fn(() => ({
    clearConsoleData: vi.fn(),
  })),
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const tagStub = defineComponent({
  setup(_, { slots }) {
    return () => h('span', slots.default?.())
  },
})

const checkboxStub = defineComponent({
  props: {
    label: {
      type: String,
      default: undefined,
    },
    value: {
      type: String,
      default: undefined,
    },
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  emits: ['update:modelValue'],
  setup(props, { slots, emit }) {
    return () => h('label', [
      h('input', {
        type: 'checkbox',
        checked: props.modelValue,
        onChange: (event: Event) => {
          emit('update:modelValue', (event.target as HTMLInputElement).checked)
        },
      }),
      slots.default?.() ?? props.label ?? props.value ?? '',
    ])
  },
})

const progressStub = defineComponent({
  props: {
    percentage: {
      type: Number,
      default: 0,
    },
  },
  setup(props) {
    return () => h('div', { class: 'progress' }, String(props.percentage))
  },
})

const emptyStub = defineComponent({
  setup() {
    return () => null
  },
})

let server: RunningGoServer
let adminLogin: LoginResponse
let userLogin: LoginResponse

async function signIn(username: string, password: string) {
  return await login({ username, password })
}

function switchSession(loginResponse: LoginResponse) {
  authState.token = loginResponse.token
  authState.isAdmin = loginResponse.user?.role === 'admin'
  userStoreState.profile = loginResponse.user ?? null
}

function mountPlanGroupsView() {
  return mount(PlanGroupsView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        EmberEmptyStateCard: passthroughStub,
        EmberFormDialog: passthroughStub,
        EmberPageHeaderCard: passthroughStub,
        EmberTableCard: passthroughStub,
        'el-checkbox': checkboxStub,
        'el-form': passthroughStub,
        'el-form-item': passthroughStub,
        'el-icon': passthroughStub,
        'el-input': passthroughStub,
        'el-input-number': passthroughStub,
        'el-progress': progressStub,
        'el-switch': passthroughStub,
        'el-table-column': emptyStub,
        'el-tag': tagStub,
        'el-tooltip': passthroughStub,
      },
    },
  })
}

function mountAccountCenterView() {
  return mount(AccountCenterView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        'el-icon': passthroughStub,
        'el-checkbox': checkboxStub,
        'el-input': passthroughStub,
        EmberFormDialog: passthroughStub,
        EmberEmptyStateCard: passthroughStub,
        DefaultAvatar: passthroughStub,
        Bell: passthroughStub,
        ChatDotRound: passthroughStub,
        Check: passthroughStub,
        CopyDocument: passthroughStub,
        Key: passthroughStub,
        Lock: passthroughStub,
        Message: passthroughStub,
        Monitor: passthroughStub,
        Reading: passthroughStub,
        UserFilled: passthroughStub,
      },
    },
  })
}

function mountSubscriptionsView() {
  return mount(SubscriptionsView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        EmberEmptyStateCard: passthroughStub,
        EmberFormDialog: passthroughStub,
        EmberPageHeaderCard: passthroughStub,
        EmberSegmentTabs: passthroughStub,
        'el-icon': passthroughStub,
        'el-tooltip': passthroughStub,
        'el-pagination': passthroughStub,
      },
    },
  })
}

function mountRankingsView() {
  return mount(RankingsView, {
    global: {
      stubs: {
        EmberPageHeaderCard: passthroughStub,
        EmberFormDialog: passthroughStub,
        EmberSegmentTabs: passthroughStub,
        EmberEmptyStateCard: passthroughStub,
        'el-icon': passthroughStub,
        'el-skeleton': passthroughStub,
        'el-empty': passthroughStub,
        'el-date-picker': passthroughStub,
        'el-checkbox-group': passthroughStub,
        'el-checkbox': checkboxStub,
        Trophy: passthroughStub,
        Film: passthroughStub,
        VideoCamera: passthroughStub,
        Calendar: passthroughStub,
        Timer: passthroughStub,
        VideoPlay: passthroughStub,
      },
    },
  })
}

describeIntegration('media library policy web+api+db flow', () => {
  beforeAll(async () => {
    server = await startGoIntegrationServer()
    request.defaults.baseURL = `${server.baseUrl}/api/v1`
    request.defaults.adapter = 'http' as never
    adminLogin = await signIn('itest_admin_web', 'integration-admin-secret')
    userLogin = await signIn('itest_user_web', 'integration-user-secret')
  }, 40000)

  afterAll(async () => {
    if (server) {
      await server.dispose()
    }
  }, 10000)

  beforeEach(() => {
    vi.clearAllMocks()
    authState.token = null
    authState.isAdmin = false
    userStoreState.profile = null
  })

  it('covers deferred template save and user apply-current sync end to end', async () => {
    switchSession(adminLogin)
    const adminWrapper = mountPlanGroupsView()
    await adminWrapper.vm.$.setupState.fetchData()
    await flushPromises()

    const vipGroup = adminWrapper.vm.$.setupState.groups.find((item: { key: string }) => item.key === 'VIP')
    expect(vipGroup, 'expected VIP group to exist').toBeTruthy()

    await adminWrapper.vm.$.setupState.openMediaDialog(vipGroup)
    await flushPromises()

    adminWrapper.vm.$.setupState.selectedLibraryIds = ['/data/movies']
    await adminWrapper.vm.$.setupState.handleSaveMediaLibraries(false)
    await flushPromises()

    const updatedVIPGroup = adminWrapper.vm.$.setupState.groups.find((item: { key: string }) => item.key === 'VIP')
    expect(updatedVIPGroup.policySyncStatus).toBe('out_of_sync')
    adminWrapper.unmount()

    switchSession(userLogin)
    const userWrapper = mountAccountCenterView()
    await userWrapper.vm.$.setupState.loadMediaLibraries()
    await flushPromises()

    expect(userWrapper.vm.$.setupState.mediaLibrarySettings.policySyncStatus).toBe('out_of_sync')
    expect(userWrapper.vm.$.setupState.mediaLibrarySettings.templateCount).toBe(1)
    expect(userWrapper.vm.$.setupState.mediaLibrarySettings.enabledCount).toBe(1)

    await userWrapper.vm.$.setupState.handleApplyCurrentMediaLibraryPolicy()
    await flushPromises()

    expect(userWrapper.vm.$.setupState.mediaLibrarySettings.policySyncStatus).toBe('synced')
    expect(userWrapper.vm.$.setupState.mediaLibrarySettings.enabledCount).toBe(1)
    userWrapper.unmount()
  })

  it('covers subscription auto approval through real web api and list refresh', async () => {
    switchSession(adminLogin)
    await updatePlanGroup('VIP', {
      subscriptionAutoApproveDailyLimit: 1,
    })

    switchSession(userLogin)
    const created = await createSubscription({
      type: 'MOVIE',
      name: 'Integration Auto Approved Movie',
      tmdbId: '900001',
      season: 0,
      confirmExisting: true,
    })
    expect(created.autoApproved).toBe(true)
    expect(created.status).toBe('APPROVED')
    expect(created.subscriptionId).toBeTruthy()

    authState.isAdmin = false
    const wrapper = mountSubscriptionsView()
    await wrapper.vm.$.setupState.fetchData()
    await flushPromises()

    const createdSubscription = wrapper.vm.$.setupState.subscriptions.find((item: { id: string }) => item.id === created.subscriptionId)
    expect(createdSubscription, 'expected created subscription to appear in list').toBeTruthy()
    expect(createdSubscription.status).toBe('APPROVED')
    wrapper.unmount()
  })

  it('covers rankings allowlist save through real web api', async () => {
    switchSession(adminLogin)
    const wrapper = mountRankingsView()
    await wrapper.vm.$.setupState.fetchRankingAllowlist(true)
    await flushPromises()

    expect(wrapper.vm.$.setupState.allowlistSummary).toBe('当前按全部媒体库统计')

    wrapper.vm.$.setupState.allowlistDialogVisible = true
    wrapper.vm.$.setupState.selectedLibraryIds = ['/data/movies']
    await wrapper.vm.$.setupState.saveRankingAllowlist()
    await flushPromises()

    expect(wrapper.vm.$.setupState.allowlistSummary).toBe('当前按 1 个媒体库统计')
    const persisted = await getRankingLibraryAllowlist()
    expect(persisted.data.allowAll).toBe(false)
    expect(persisted.data.libraryIds).toEqual(['/data/movies'])
    wrapper.unmount()
  })
})
