import { defineComponent, h } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UsersView from './UsersView.vue'
import UserEntitlementsPanel from '@/components/billing/UserEntitlementsPanel.vue'
import {
  applyAdminUserCurrentPolicySync,
  applyPlanGroupMediaLibrarySync,
  getAdminMediaLibraries,
  getPlanGroups,
  getUsers,
  previewPlanGroupMediaLibrarySync,
  updateAdminUser
} from '@/api/admin'
import { getEntitlements } from '@/api/entitlements'
import type { UserInfo } from '@/types/api'

vi.mock('@/api/entitlements', () => ({ getEntitlements: vi.fn() }))

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

const routerPush = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPush,
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
        'el-input-number': emptyStub,
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
    vi.mocked(getEntitlements).mockResolvedValue({ data: [], businessTimezone: 'Asia/Shanghai' })
    vi.mocked(getPlanGroups).mockResolvedValue({ data: [] })
    vi.mocked(getAdminMediaLibraries).mockResolvedValue({ data: [] })
    vi.mocked(getUsers).mockResolvedValue({
      data: [],
      total: 0,
      page: 1,
      pageSize: 10,
    })
  })

  it('权益保存后刷新打开的弹窗当前分组，无需关闭重开', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { entitlementUser: UserInfo | null }
    vm.entitlementUser = createUser({ planGroup: 'BASE' })
    await flushPromises()
    const panel = wrapper.findComponent(UserEntitlementsPanel)
    expect(panel.props('currentGroup')).toBe('BASE')
    vi.mocked(getUsers).mockResolvedValueOnce({ data: [createUser({ planGroup: 'PLUS' })], total: 1, page: 1, pageSize: 10 })

    panel.vm.$emit('changed')
    await flushPromises()

    expect(wrapper.findComponent(UserEntitlementsPanel).props('currentGroup')).toBe('PLUS')
    wrapper.unmount()
  })

  it('权益变更后用户不再匹配当前筛选时关闭旧弹窗', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { entitlementUser: UserInfo | null }
    vm.entitlementUser = createUser({ planGroup: 'BASE' })
    await flushPromises()

    wrapper.findComponent(UserEntitlementsPanel).vm.$emit('changed')
    await flushPromises()

    expect(vm.entitlementUser).toBeNull()
    expect(wrapper.findComponent(UserEntitlementsPanel).exists()).toBe(false)
    wrapper.unmount()
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

  it('持有权益组独立筛选并重置页码，重置清空两个分组条件', async () => {
    const wrapper = await mountView()
    const field = wrapper.find('[label="持有权益组"]')
    expect(field.exists()).toBe(true)
    expect(wrapper.find('[label="当前分组"]').exists()).toBe(true)
    const vm = wrapper.vm as unknown as {
      queryParams: { page: number; planGroup: string; entitlementGroup: string }
      handleFilterChange: () => void
      handleResetFilters: () => void
      selectedFilterPlanGroup: unknown
    }
    vm.queryParams.page = 3
    vm.queryParams.entitlementGroup = 'SECOND'
    vm.handleFilterChange()
    await flushPromises()
    expect(getUsers).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, planGroup: '', entitlementGroup: 'SECOND' }))
    expect(vm.selectedFilterPlanGroup).toBeNull()
    vm.queryParams.planGroup = 'DEFAULT'
    vm.handleFilterChange()
    await flushPromises()
    expect(getUsers).toHaveBeenLastCalledWith(expect.objectContaining({ planGroup: 'DEFAULT', entitlementGroup: 'SECOND' }))
    vm.handleResetFilters()
    await flushPromises()
    expect(getUsers).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, planGroup: '', entitlementGroup: '' }))
    wrapper.unmount()
  })

  it('同步批次入口跳到计费中心的套餐分组并保留批次 ID', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      handleViewPolicySyncBatch: (row: UserInfo) => void
    }

    vm.handleViewPolicySyncBatch(createUser({ policySyncBatchId: 'batch_1' }))

    expect(routerPush).toHaveBeenCalledWith({
      name: 'console-billing',
      query: {
        tab: 'groups',
        syncBatchId: 'batch_1',
      },
    })
  })

  it('编辑用户时非法到期时间按无到期时间处理，不抛 RangeError', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      editExpiryAction: string; editForm: { expiresAt: string | null }
      handleOpenEdit: (row: UserInfo) => void
    }

    expect(() => vm.handleOpenEdit(createUser({ expiresAt: 'not-a-date' }))).not.toThrow()
    expect(vm.editForm.expiresAt).toBeNull()
    expect(vm.editExpiryAction).toBe('keep')
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

  it('换组冲突保留编辑内容，不向 Vue 事件处理器抛出已提示的错误', async () => {
    vi.mocked(updateAdminUser).mockRejectedValueOnce(new Error('target group already owned'))
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { editForm: { planGroup: string }; editDialogVisible: boolean; savingUser: boolean; handleOpenEdit: (row: UserInfo) => void; handleUpdateUser: () => Promise<void> }
    vm.handleOpenEdit(createUser({ planGroup: 'FIRST', effectivePlanGroup: 'FIRST' }))
    vm.editForm.planGroup = 'DEFAULT'
    await expect(vm.handleUpdateUser()).resolves.toBeUndefined()
    expect(vm.editDialogVisible).toBe(true)
    expect(vm.editForm.planGroup).toBe('DEFAULT')
    expect(vm.savingUser).toBe(false)
    wrapper.unmount()
  })

  it.each(['2099-01-01T00:00:00Z', '2020-01-01T00:00:00Z', undefined])('编辑分组不发送或重算期限：%s', async (expiresAt) => {
    vi.mocked(updateAdminUser).mockResolvedValue(createUser())
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { editForm: { planGroup: string }; handleOpenEdit: (row: UserInfo) => void; handleUpdateUser: () => Promise<void> }
    vm.handleOpenEdit(createUser({ planGroup: 'FIRST', effectivePlanGroup: 'FIRST', expiresAt }))
    vm.editForm.planGroup = 'DEFAULT'
    await vm.handleUpdateUser()
    expect(updateAdminUser).toHaveBeenCalledWith('user_1', { planGroup: 'DEFAULT' })
    wrapper.unmount()
  })

  it('编辑当前分组可设永久和业务时区的指定到期时间', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { editExpiryAction: string; editForm: { expiresAt: string | null }; handleOpenEdit: (row: UserInfo) => Promise<void>; handleUpdateUser: () => Promise<void> }
    await vm.handleOpenEdit(createUser({ planGroup: 'VIP', expiresAt: '2099-01-01T00:00:00Z' }))
    expect(vm.editForm.expiresAt).toBe('2099-01-01 08:00:00')
    vm.editExpiryAction = 'permanent'
    await vm.handleUpdateUser()
    expect(updateAdminUser).toHaveBeenLastCalledWith('user_1', { clearExpiresAt: true })
    await vm.handleOpenEdit(createUser({ planGroup: 'VIP', expiresAt: undefined }))
    vm.editExpiryAction = 'set'; vm.editForm.expiresAt = '2099-02-01 12:00:00'
    await vm.handleUpdateUser()
    expect(updateAdminUser).toHaveBeenLastCalledWith('user_1', { expiresAt: '2099-02-01 12:00:00' })
    wrapper.unmount()
  })

  it('日期未填写或同时换组改期限时不提交，避免误覆盖其他组', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { editExpiryAction: string; editForm: { expiresAt: string | null; planGroup: string }; handleOpenEdit: (row: UserInfo) => Promise<void>; handleUpdateUser: () => Promise<void> }
    await vm.handleOpenEdit(createUser({ planGroup: 'VIP' }))
    vm.editExpiryAction = 'set'; vm.editForm.expiresAt = null
    await vm.handleUpdateUser()
    expect(updateAdminUser).not.toHaveBeenCalled()
    vm.editExpiryAction = 'permanent'; vm.editForm.planGroup = 'BASE'
    await vm.handleUpdateUser()
    expect(updateAdminUser).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('编辑默认保持期限不变，延期与资料一次提交，失败重试不更换操作号', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { editExpiryAction: string; editExtendDays: number; editForm: { email: string }; handleOpenEdit: (row: UserInfo) => Promise<void>; handleUpdateUser: () => Promise<void> }
    await vm.handleOpenEdit(createUser({ planGroup: 'VIP' }))
    expect(vm.editExpiryAction).toBe('keep')
    expect(wrapper.find('[aria-label="延长有效期"]').exists()).toBe(false)
    vm.editForm.email = 'changed@example.com'
    vm.editExpiryAction = 'extend'; vm.editExtendDays = 45
    vi.mocked(updateAdminUser).mockRejectedValueOnce(new Error('response lost'))
    await vm.handleUpdateUser()
    const first = vi.mocked(updateAdminUser).mock.calls[0]![1]
    expect(first).toEqual({ email: 'changed@example.com', extendDays: 45, operationId: expect.any(String) })
    await vm.handleUpdateUser()
    expect(updateAdminUser).toHaveBeenLastCalledWith('user_1', first)
    wrapper.unmount()
  })

  it('延期天数无效不提交，重新打开恢复保持不变', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { editExpiryAction: string; editExtendDays: number; handleOpenEdit: (row: UserInfo) => Promise<void>; handleUpdateUser: () => Promise<void> }
    await vm.handleOpenEdit(createUser({ planGroup: 'VIP' }))
    vm.editExpiryAction = 'extend'; vm.editExtendDays = 0
    await vm.handleUpdateUser()
    expect(updateAdminUser).not.toHaveBeenCalled()
    await vm.handleOpenEdit(createUser({ planGroup: 'VIP' }))
    expect(vm.editExpiryAction).toBe('keep')
    wrapper.unmount()
  })

  it('历史同步不一致时提交模板集合和偏好用户', async () => {
    vi.mocked(getPlanGroups).mockResolvedValue({
      data: [{
        key: 'VIP',
        name: 'VIP',
        isDefault: false,
        sortOrder: 1,
        p115PlaybackMode: 'personal',
        p115TransferHourlyLimit: 5,
        p115TransferDailyLimit: 10,
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
