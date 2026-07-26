import { defineComponent, h, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PlanGroupsView from './PlanGroupsView.vue'
import type { EmbyPolicySyncBatchDetail, ManagedPlanGroup } from '@/types/api'
import {
  createPlanGroup,
  deletePlanGroup,
  getAdminMediaLibraries,
  getEmbyPolicySyncBatch,
  getPlanGroupEmbyPolicyTemplate,
  getPlanGroupMediaLibraries,
  getPlanGroups,
  retryFailedEmbyPolicySyncBatch,
  updatePlanGroup,
  updatePlanGroupEmbyPolicyTemplate,
  updatePlanGroupMediaLibraries,
} from '@/api/admin'
import { ElMessage, ElMessageBox } from 'element-plus'

const routeState = vi.hoisted(() => ({
  route: {
    query: {} as Record<string, string | string[] | undefined>,
  },
}))

vi.mock('@/api/admin', () => ({
  createPlanGroup: vi.fn(),
  deletePlanGroup: vi.fn(),
  getAdminMediaLibraries: vi.fn(),
  getEmbyPolicySyncBatch: vi.fn(),
  getPlanGroupEmbyPolicyTemplate: vi.fn(),
  getPlanGroupMediaLibraries: vi.fn(),
  getPlanGroups: vi.fn(),
  retryFailedEmbyPolicySyncBatch: vi.fn(),
  updatePlanGroup: vi.fn(),
  updatePlanGroupEmbyPolicyTemplate: vi.fn(),
  updatePlanGroupMediaLibraries: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
  ElMessageBox: {
    confirm: vi.fn(),
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState.route,
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
  },
  setup(props, { slots }) {
    return () => h('span', { class: 'checkbox' }, slots.default?.() ?? props.label ?? '')
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

function createBatch(overrides: Partial<EmbyPolicySyncBatchDetail> = {}): EmbyPolicySyncBatchDetail {
  return {
    id: 'batch_failed',
    planGroupKey: 'vip',
    reason: 'plan_group_policy_template',
    status: 'failed',
    totalCount: 2,
    pendingCount: 0,
    processingCount: 0,
    syncedCount: 0,
    failedCount: 2,
    failedUsers: [
      {
        userId: 'user_1',
        username: 'alice',
        embyId: 'emby_1',
        error: 'Emby 写入失败',
      },
      {
        userId: 'user_2',
        username: 'bob',
        embyId: 'emby_2',
        error: 'Policy 不一致',
      },
    ],
    createdAt: '2026-05-29T00:00:00Z',
    updatedAt: '2026-05-29T00:00:00Z',
    finishedAt: '2026-05-29T00:00:00Z',
    ...overrides,
  }
}

// 视图在测试中被直接驱动的 setup 内部成员（ref 已解包）。
type PlanGroupsSetupState = {
  activeSyncBatch: EmbyPolicySyncBatchDetail | null
  selectedGroup: ManagedPlanGroup | null
  selectedLibraryIds: string[]
  syncPollErrorCount: number
  createForm: {
    key: string
    name: string
    description: string
    isDefault: boolean
    sortOrder: number
    subscriptionAutoApproveDailyLimit: number
  }
  openMediaDialog: (group: ManagedPlanGroup) => Promise<void>
  canDeletePlanGroup: (group: ManagedPlanGroup) => boolean
  handleCreate: () => Promise<void>
  handleDelete: (group: ManagedPlanGroup) => Promise<void>
  handleSaveMediaLibraries: (applyToExistingUsers?: boolean) => Promise<void>
  pollSyncBatch: (batchId: string) => Promise<void>
}

/**
 * 读取组件 setup 内部状态。
 * Vue 3.5 起 setupState 不再出现在公开的 ComponentInternalInstance 类型上（运行时仍存在），
 * 测试需要直接驱动组件内部方法与状态，这里以结构化类型收窄出实际用到的成员。
 */
function setupStateOf(wrapper: VueWrapper): PlanGroupsSetupState {
  return (wrapper.vm.$ as unknown as { setupState: PlanGroupsSetupState }).setupState
}

function mountView() {
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

describe('PlanGroupsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeState.route.query = {}
    vi.mocked(getPlanGroups).mockResolvedValue({ data: [] })
    vi.mocked(getAdminMediaLibraries).mockResolvedValue({ data: [] })
    vi.mocked(getPlanGroupMediaLibraries).mockResolvedValue({ data: null } as never)
    vi.mocked(getPlanGroupEmbyPolicyTemplate).mockResolvedValue({ data: null } as never)
    vi.mocked(createPlanGroup).mockResolvedValue({} as never)
    vi.mocked(deletePlanGroup).mockResolvedValue({} as never)
    vi.mocked(updatePlanGroup).mockResolvedValue({} as never)
    vi.mocked(updatePlanGroupMediaLibraries).mockResolvedValue({
      data: { mode: 'batch', batchId: 'batch_empty', affectedUserCount: 0, status: 'synced' },
    })
    vi.mocked(updatePlanGroupEmbyPolicyTemplate).mockResolvedValue({
      data: { batchId: 'batch_empty', affectedUserCount: 0, status: 'synced' },
    })
  })

  it('失败同步批次展示失败用户摘要并允许重试失败项', async () => {
    vi.mocked(retryFailedEmbyPolicySyncBatch).mockResolvedValue({
      data: { batchId: 'batch_retry', affectedUserCount: 2, status: 'pending' },
    })
    vi.mocked(getEmbyPolicySyncBatch).mockResolvedValue({
      data: createBatch({
        id: 'batch_retry',
        status: 'pending',
        pendingCount: 2,
        failedCount: 0,
        failedUsers: [],
        finishedAt: null,
      }),
    })

    const wrapper = mountView()
    await flushPromises()

    setupStateOf(wrapper).activeSyncBatch = createBatch()
    await nextTick()

    expect(wrapper.text()).toContain('重试失败项')
    expect(wrapper.text()).toContain('失败用户')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('Emby 写入失败')

    const retryButton = wrapper.findAll('button').find(item => item.text().includes('重试失败项'))
    expect(retryButton, '未找到重试失败项按钮').toBeTruthy()
    await retryButton!.trigger('click')
    await flushPromises()

    expect(retryFailedEmbyPolicySyncBatch).toHaveBeenCalledWith('batch_failed')
    expect(getEmbyPolicySyncBatch).toHaveBeenCalledWith('batch_retry')
    expect(ElMessage.success).toHaveBeenCalledWith('已提交 2 个失败项重试')

    wrapper.unmount()
  })

  it('从同步批次链接进入时直接展示对应批次详情', async () => {
    routeState.route.query = { syncBatchId: 'batch_failed' }
    vi.mocked(getEmbyPolicySyncBatch).mockResolvedValue({
      data: createBatch(),
    })

    const wrapper = mountView()
    await flushPromises()

    expect(getEmbyPolicySyncBatch).toHaveBeenCalledWith('batch_failed')
    expect(wrapper.text()).toContain('Emby Policy 同步')
    expect(wrapper.text()).toContain('失败用户')
    expect(wrapper.text()).toContain('alice')

    wrapper.unmount()
  })

  it('媒体库模板只展示媒体库名称，不暴露媒体库 ID', async () => {
    vi.mocked(getAdminMediaLibraries).mockResolvedValue({
      data: [
        { id: 'lib_movie_internal_id', name: '电影', type: 'Movie' },
        { id: 'lib_series_internal_id', name: '剧集', type: 'Series' },
      ],
    })
    vi.mocked(getPlanGroupMediaLibraries).mockResolvedValue({
      data: {
        planGroupKey: 'vip',
        planGroupName: 'VIP',
        libraries: [{ id: 'lib_movie_internal_id', name: '电影', type: 'Movie' }],
        libraryCount: 1,
        affectedUserCount: 0,
      },
    })

    const wrapper = mountView()
    await flushPromises()

    await setupStateOf(wrapper).openMediaDialog({
      key: 'vip',
      name: 'VIP',
      description: '',
      isDefault: false,
      sortOrder: 0,
    })
    await flushPromises()

    expect(wrapper.text()).toContain('电影')
    expect(wrapper.text()).toContain('剧集')
    expect(wrapper.text()).toContain('Movie')
    expect(wrapper.text()).not.toContain('0 项')
    expect(wrapper.text()).not.toContain('lib_movie_internal_id')
    expect(wrapper.text()).not.toContain('lib_series_internal_id')

    wrapper.unmount()
  })

  it('默认分组不允许触发删除', async () => {
    const wrapper = mountView()
    await flushPromises()

    const defaultGroup = {
      key: 'DEFAULT',
      name: '默认分组',
      description: '系统默认套餐分组',
      isDefault: true,
      sortOrder: 0,
    }

    expect(setupStateOf(wrapper).canDeletePlanGroup(defaultGroup)).toBe(false)

    await setupStateOf(wrapper).handleDelete(defaultGroup)
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('默认分组不能删除')
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(deletePlanGroup).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('创建分组时提交自动通过额度字段', async () => {
    const wrapper = mountView()
    await flushPromises()

    setupStateOf(wrapper).createForm = {
      key: 'VIP_A',
      name: 'VIP A',
      description: '高级分组',
      isDefault: false,
      sortOrder: 10,
      subscriptionAutoApproveDailyLimit: 2,
    }

    await setupStateOf(wrapper).handleCreate()
    await flushPromises()

    expect(createPlanGroup).toHaveBeenCalledWith({
      key: 'VIP_A',
      name: 'VIP A',
      description: '高级分组',
      isDefault: false,
      sortOrder: 10,
      subscriptionAutoApproveDailyLimit: 2,
    })

    wrapper.unmount()
  })

  it('仅保存媒体库模板时不启动批次轮询并提示待同步用户数', async () => {
    vi.mocked(updatePlanGroupMediaLibraries).mockResolvedValueOnce({
      data: {
        mode: 'deferred',
        affectedUserCount: 3,
        outOfSyncUserCount: 3,
        status: 'out_of_sync',
      },
    })

    const wrapper = mountView()
    await flushPromises()

    setupStateOf(wrapper).selectedGroup = {
      key: 'vip',
      name: 'VIP',
      description: '',
      isDefault: false,
      sortOrder: 0,
    }
    setupStateOf(wrapper).selectedLibraryIds = ['lib_movie']

    await setupStateOf(wrapper).handleSaveMediaLibraries(false)
    await flushPromises()

    expect(updatePlanGroupMediaLibraries).toHaveBeenCalledWith('vip', ['lib_movie'], false)
    expect(ElMessage.success).toHaveBeenCalledWith('模板已保存，3 个用户待同步')
    expect(setupStateOf(wrapper).activeSyncBatch).toBeNull()

    wrapper.unmount()
  })

  it('轮询遇瞬时错误不立即停摆，连续超过上限才停止', async () => {
    vi.mocked(getEmbyPolicySyncBatch).mockRejectedValueOnce(new Error('network'))
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({
        data: createBatch({ status: 'synced', syncedCount: 2, failedCount: 0, failedUsers: [] }),
      })

    const wrapper = mountView()
    await flushPromises()

    // 预置一个非终态批次，验证瞬时错误不会清空已展示的批次摘要。
    setupStateOf(wrapper).activeSyncBatch = createBatch({ status: 'pending' })
    await nextTick()

    // 第一次、第二次失败：累计错误计数，但批次状态保持。
    await setupStateOf(wrapper).pollSyncBatch('batch_failed')
    expect(setupStateOf(wrapper).syncPollErrorCount).toBe(1)
    expect(setupStateOf(wrapper).activeSyncBatch).not.toBeNull()

    await setupStateOf(wrapper).pollSyncBatch('batch_failed')
    expect(setupStateOf(wrapper).syncPollErrorCount).toBe(2)

    // 第三次成功：错误计数清零，终态正常推进。
    await setupStateOf(wrapper).pollSyncBatch('batch_failed')
    await flushPromises()

    expect(setupStateOf(wrapper).syncPollErrorCount).toBe(0)
    expect(ElMessage.success).toHaveBeenCalledWith('用户同步已完成')

    wrapper.unmount()
  })
})
