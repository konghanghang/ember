import { defineComponent, h } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UsersView from './UsersView.vue'
import {
  applyAdminUserCurrentPolicySync,
  applyPlanGroupMediaLibrarySync,
  getAdminMediaLibraries,
  getPlanGroups,
  getUsers,
  previewPlanGroupMediaLibrarySync,
  updateAdminUser
} from '@/api/admin'
import type { UserInfo } from '@/types/api'

vi.mock('@/api/admin', () => ({
  applyAdminUserCurrentPolicySync: vi.fn(),
  applyPlanGroupMediaLibrarySync: vi.fn(),
  clearAdminUserMediaLibraryPreferences: vi.fn(),
  createAdminUser: vi.fn(),
  deleteUser: vi.fn(),
  extendUserExpiry: vi.fn(),
  getAdminMediaLibraries: vi.fn(),
  getPlanGroups: vi.fn(),
  getUsers: vi.fn(),
  previewPlanGroupMediaLibrarySync: vi.fn(),
  resetUserPassword: vi.fn(),
  syncAdminUserMediaLibraryPreferences: vi.fn(),
  toggleUserStatus: vi.fn(),
  updateAdminUser: vi.fn(),
  updateAdminUserEmbyAccess: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    warning: vi.fn(),
  },
  ElMessageBox: {
    alert: vi.fn(),
    confirm: vi.fn(),
    prompt: vi.fn(),
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const emptyStub = defineComponent({
  setup() {
    return () => null
  },
})

function createUser(overrides: Partial<UserInfo> = {}): UserInfo {
  return {
    id: 'user_1',
    username: 'alice',
    role: 'user',
    email: 'alice@example.com',
    embyId: 'emby_1',
    embyAccessDisabled: false,
    embyDisabled: true,
    isExpired: false,
    expiresAt: '2099-01-01T00:00:00Z',
    isActive: true,
    createdAt: '2026-05-30T00:00:00Z',
    ...overrides,
  }
}

async function mountView() {
  const wrapper = shallowMount(UsersView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        DefaultAvatar: emptyStub,
        EmberDateField: passthroughStub,
        EmberFilterPanel: passthroughStub,
        EmberFormDialog: passthroughStub,
        EmberPageHeaderCard: passthroughStub,
        EmberSearchInput: passthroughStub,
        EmberSelectField: passthroughStub,
        EmberTableCard: passthroughStub,
        'el-date-picker': emptyStub,
        'el-dropdown': passthroughStub,
        'el-dropdown-item': passthroughStub,
        'el-dropdown-menu': passthroughStub,
        'el-form': passthroughStub,
        'el-form-item': passthroughStub,
        'el-icon': passthroughStub,
        'el-checkbox': emptyStub,
        'el-input': emptyStub,
        'el-option': emptyStub,
        'el-pagination': emptyStub,
        'el-select': passthroughStub,
        'el-switch': emptyStub,
        'el-table-column': emptyStub,
        'el-tag': passthroughStub,
        'el-tooltip': passthroughStub,
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('UsersView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getPlanGroups).mockResolvedValue({ data: [] })
    vi.mocked(getAdminMediaLibraries).mockResolvedValue({ data: [] })
    vi.mocked(getUsers).mockResolvedValue({
      data: [],
      total: 0,
      page: 1,
      pageSize: 10,
    })
  })

  it('不会把 Ember 本地账号禁用解释成 Emby 禁用来源', async () => {
    const wrapper = await mountView()

    const status = (wrapper.vm as unknown as {
      getEmbyStatus: (row: UserInfo) => { reason: string; text: string }
    }).getEmbyStatus(createUser({ isActive: false }))

    expect(status.text).toBe('禁用')
    expect(status.reason).toBe('手动/异常禁用')
  })

  it('Emby 禁用原因优先按管理员禁用和过期解释', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      getEmbyStatus: (row: UserInfo) => { reason: string; text: string }
    }

    expect(vm.getEmbyStatus(createUser({
      embyAccessDisabled: true,
      isActive: false,
    })).reason).toBe('管理员禁用')

    expect(vm.getEmbyStatus(createUser({
      isActive: false,
      isExpired: true,
    })).reason).toBe('过期封禁')
  })

  it('切换每页条数时重置到第 1 页再请求', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      queryParams: { page: number; pageSize: number }
      handlePageSizeChange: (size: number) => void
    }

    vm.queryParams.page = 5
    vm.handlePageSizeChange(50)
    await flushPromises()

    expect(vm.queryParams.page).toBe(1)
    expect(vm.queryParams.pageSize).toBe(50)
    expect(getUsers).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, pageSize: 50 }))
  })

  it('编辑用户时非法到期时间按无到期时间处理，不抛 RangeError', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      editForm: { expiresAt: Date | null; neverExpire: boolean }
      handleOpenEdit: (row: UserInfo) => void
    }

    expect(() => vm.handleOpenEdit(createUser({ expiresAt: 'not-a-date' }))).not.toThrow()
    expect(vm.editForm.expiresAt).toBeNull()
    expect(vm.editForm.neverExpire).toBe(false)
  })

  it('编辑用户时只提交实际变更字段', async () => {
    vi.mocked(updateAdminUser).mockResolvedValue(createUser())
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      editForm: { email: string }
      handleOpenEdit: (row: UserInfo) => void
      handleUpdateUser: () => Promise<void>
    }

    vm.handleOpenEdit(createUser({
      email: 'alice@example.com',
      planGroup: 'VIP',
      effectivePlanGroup: 'VIP',
      expiresAt: '2099-01-01T00:00:00Z',
      isActive: false,
    }))
    vm.editForm.email = 'alice.new@example.com'

    await vm.handleUpdateUser()
    await flushPromises()

    expect(updateAdminUser).toHaveBeenCalledWith('user_1', {
      email: 'alice.new@example.com',
    })
  })

  it('历史同步不一致时提交模板集合和偏好用户', async () => {
    vi.mocked(getPlanGroups).mockResolvedValue({
      data: [{
        key: 'VIP',
        name: 'VIP',
        isDefault: false,
        sortOrder: 1,
      }],
    })
    vi.mocked(getAdminMediaLibraries).mockResolvedValue({
      data: [
        { id: 'lib_a', name: '电影', type: 'Movie' },
        { id: 'lib_b', name: '剧集', type: 'Series' },
      ],
    })
    vi.mocked(previewPlanGroupMediaLibrarySync).mockResolvedValue({
      data: {
        planGroupKey: 'VIP',
        totalUsers: 2,
        scannedUsers: 2,
        consistent: false,
        candidates: [
          {
            libraryIds: ['lib_a'],
            libraries: [{ id: 'lib_a', name: '电影', type: 'Movie' }],
            userCount: 1,
            sourceUserIds: ['user_1'],
          },
          {
            libraryIds: ['lib_b'],
            libraries: [{ id: 'lib_b', name: '剧集', type: 'Series' }],
            userCount: 1,
            sourceUserIds: ['user_2'],
          },
        ],
        differenceUsers: [
          {
            userId: 'user_1',
            username: 'alice',
            embyId: 'emby_1',
            libraryIds: ['lib_a'],
            libraries: [{ id: 'lib_a', name: '电影', type: 'Movie' }],
          },
          {
            userId: 'user_2',
            username: 'bob',
            embyId: 'emby_2',
            libraryIds: ['lib_b'],
            libraries: [{ id: 'lib_b', name: '剧集', type: 'Series' }],
          },
        ],
        failedItems: [],
      },
    })
    vi.mocked(applyPlanGroupMediaLibrarySync).mockResolvedValue({
      data: {
        batchId: 'batch_1',
        affectedUserCount: 2,
        status: 'pending',
      },
    })

    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      queryParams: { planGroup: string }
      selectedSyncLibraryIds: string[]
      selectedPreferenceUserIds: string[]
      handleSyncHistoryLibraries: () => Promise<void>
      handleApplyHistoryLibraries: () => Promise<void>
    }

    vm.queryParams.planGroup = 'VIP'
    await vm.handleSyncHistoryLibraries()

    expect(vm.selectedSyncLibraryIds).toEqual(['lib_a'])
    expect(vm.selectedPreferenceUserIds).toEqual(['user_2'])

    await vm.handleApplyHistoryLibraries()

    expect(applyPlanGroupMediaLibrarySync).toHaveBeenCalledWith('VIP', {
      libraryIds: ['lib_a'],
      preferenceUserIds: ['user_2'],
    })
  })

  it('管理员可以对单个用户触发同步到 Emby', async () => {
    vi.mocked(applyAdminUserCurrentPolicySync).mockResolvedValue({ data: createUser() })

    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      handleApplyCurrentPolicySync: (row: UserInfo) => Promise<void>
    }

    await vm.handleApplyCurrentPolicySync(createUser({
      policySyncStatus: 'out_of_sync',
    }))
    await flushPromises()

    expect(applyAdminUserCurrentPolicySync).toHaveBeenCalledWith('user_1')
  })
})
