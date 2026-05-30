import { defineComponent, h } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UsersView from './UsersView.vue'
import { getPlanGroups, getUsers, updateAdminUser } from '@/api/admin'
import type { UserInfo } from '@/types/api'

vi.mock('@/api/admin', () => ({
  applyPlanGroupMediaLibrarySync: vi.fn(),
  clearAdminUserMediaLibraryPreferences: vi.fn(),
  createAdminUser: vi.fn(),
  deleteUser: vi.fn(),
  extendUserExpiry: vi.fn(),
  getPlanGroups: vi.fn(),
  getUsers: vi.fn(),
  previewPlanGroupMediaLibrarySync: vi.fn(),
  resetUserPassword: vi.fn(),
  retryAdminUserPolicySync: vi.fn(),
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
    vi.mocked(getUsers).mockResolvedValue({
      data: [],
      total: 0,
      page: 1,
      pageSize: 10,
      totalPages: 0,
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
})
