import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SubscriptionsView from './SubscriptionsView.vue'
import type { Subscription, SubscriptionManualCandidate, SubscriptionManualSearchResult } from '@/types/api'
import {
  manualDispatchSubscription,
  manualSearchSubscription
} from '@/api/admin'
import { getSubscriptions, resubmitSubscription } from '@/api/console'
import { ElMessage, ElMessageBox } from 'element-plus'

vi.mock('@/api/admin', () => ({
  approveSubscription: vi.fn(),
  rejectSubscription: vi.fn(),
  markSubscriptionIngested: vi.fn(),
  redispatchSubscription: vi.fn(),
  deleteSubscriptionAsAdmin: vi.fn(),
  manualSearchSubscription: vi.fn(),
  manualDispatchSubscription: vi.fn(),
}))

vi.mock('@/api/console', () => ({
  getSubscriptions: vi.fn(),
  deleteSubscription: vi.fn(),
  resubmitSubscription: vi.fn(),
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
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
  ElMessageBox: {
    confirm: vi.fn(),
    prompt: vi.fn(),
  },
}))

const authStoreState = reactive({
  isAdmin: true,
})

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(() => ({
    get isAdmin() {
      return authStoreState.isAdmin
    },
  })),
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const EmberFormDialogStub = defineComponent({
  props: {
    modelValue: { type: Boolean, default: false },
    title: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(props, { slots }) {
    return () => {
      if (!props.modelValue) {
        return null
      }
      return h(
        'div',
        {
          'data-test': 'manual-dialog',
          'data-title': props.title,
        },
        [
          h('div', { 'data-test': 'dialog-body' }, slots.default?.()),
          h('div', { 'data-test': 'dialog-footer' }, slots.footer?.()),
        ],
      )
    }
  },
})

const ElInputNumberStub = defineComponent({
  props: {
    modelValue: { type: [Number, null] as any, default: null },
    min: { type: Number, default: undefined },
    step: { type: Number, default: undefined },
    precision: { type: Number, default: undefined },
    disabled: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        type: 'number',
        'data-test': 'season-input',
        'data-disabled': props.disabled ? 'true' : 'false',
        value: props.modelValue ?? '',
        onInput: (event: Event) => {
          const raw = (event.target as HTMLInputElement).value
          const parsed = raw === '' ? null : Number(raw)
          emit('update:modelValue', parsed)
        },
      })
  },
})

function buildSubscription(overrides: Partial<Subscription> = {}): Subscription {
  return {
    id: 'sub_1',
    userId: 'u1',
    type: 'TV',
    name: 'Demo Show',
    tmdbId: '12345',
    season: 0,
    status: 'APPROVED',
    createdAt: '2026-01-01T00:00:00Z',
    user: { username: 'demo', email: 'demo@example.com' },
    ...overrides,
  }
}

function mountView() {
  return mount(SubscriptionsView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        'el-icon': passthroughStub,
        'el-input-number': ElInputNumberStub,
        EmberFormDialog: EmberFormDialogStub,
        EmberPageHeaderCard: passthroughStub,
        EmberSegmentTabs: passthroughStub,
        EmberEmptyStateCard: passthroughStub,
        Check: passthroughStub,
        Close: passthroughStub,
        Delete: passthroughStub,
        Download: passthroughStub,
        Plus: passthroughStub,
        Refresh: passthroughStub,
        RefreshRight: passthroughStub,
        Search: passthroughStub,
        VideoPlay: passthroughStub,
        Film: passthroughStub,
        'el-tooltip': true,
        'el-pagination': true,
      },
    },
  })
}

async function openManualDialog(wrapper: ReturnType<typeof mountView>, sub: any) {
  // 卡片操作按钮由 cardActionButtons 生成，整剧 APPROVED 订阅包含"手动下载"按钮。
  const button = wrapper
    .findAll('button')
    .find((item) => item.text().includes('手动下载'))
  expect(button, '未找到手动下载按钮').toBeTruthy()
  await button!.trigger('click')
  await flushPromises()
  return sub
}

function findSeasonInput(wrapper: ReturnType<typeof mountView>) {
  return wrapper.find('[data-test="season-input"]')
}

function findSearchButton(wrapper: ReturnType<typeof mountView>) {
  const button = wrapper
    .findAll('[data-test="dialog-body"] button')
    .find((item) => item.text().includes('搜索候选') || item.text().includes('搜索中'))
  expect(button, '未找到搜索候选按钮').toBeTruthy()
  return button!
}

function findDispatchButton(wrapper: ReturnType<typeof mountView>) {
  const button = wrapper
    .findAll('[data-test="dialog-footer"] button')
    .find((item) => item.text().includes('确认下发') || item.text().includes('下发中'))
  expect(button, '未找到确认下发按钮').toBeTruthy()
  return button!
}

function resolveSearchResponse(candidates: SubscriptionManualCandidate[]): { data: SubscriptionManualSearchResult } {
  return {
    data: {
      subscription: buildSubscription(),
      source: 'Demo Site',
      query: 'Demo Show',
      matchMode: 'normal',
      candidates,
    },
  }
}

describe('SubscriptionsView 手动下载', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    authStoreState.isAdmin = true
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [],
      total: 0,
    })
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm' as never)
  })

  it('电影订阅打开弹窗时自动搜索，下发只透传 candidate 不带 candidateId', async () => {
    const movie = buildSubscription({
      id: 'sub_movie',
      type: 'MOVIE',
      season: 0,
      name: 'Demo Movie',
    })
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [movie],
      total: 1,
    })
    vi.mocked(manualSearchSubscription).mockResolvedValue(
      resolveSearchResponse([
        {
          id: 'c1',
          title: 'Demo Movie 1080p',
          site: 'Site A',
          size: 1024 * 1024 * 1024 * 2,
          seeders: 5,
          payload: { torrent_download: 'link' },
        },
      ]),
    )
    vi.mocked(manualDispatchSubscription).mockResolvedValue({
      data: { subscription: movie, accepted: true },
    })

    const wrapper = mountView()
    await flushPromises()

    await openManualDialog(wrapper, movie)
    await flushPromises()

    expect(manualSearchSubscription).toHaveBeenCalledWith('sub_movie', {})
    // 确认下发
    await findDispatchButton(wrapper).trigger('click')
    await flushPromises()

    expect(manualDispatchSubscription).toHaveBeenCalledWith('sub_movie', {
      candidate: expect.objectContaining({ id: 'c1' }),
    })
    // 请求体不应再带 candidateId
    const callArgs = vi.mocked(manualDispatchSubscription).mock.calls[0][1] as any
    expect(callArgs.candidateId).toBeUndefined()
    expect(callArgs.season).toBeUndefined()
  })

  it('整剧订阅确认下发时携带搜索时的季号', async () => {
    const series = buildSubscription({
      id: 'sub_series_dispatch',
      type: 'TV',
      season: 0,
      name: 'Demo Show',
    })
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [series],
      total: 1,
    })
    vi.mocked(manualSearchSubscription).mockResolvedValue(
      resolveSearchResponse([
        {
          id: 'c1',
          title: 'Demo Show S02 1080p',
          site: 'Site A',
          payload: { torrent_download: 'link' },
        },
      ]),
    )
    vi.mocked(manualDispatchSubscription).mockResolvedValue({
      data: { subscription: series, accepted: true },
    })

    const wrapper = mountView()
    await flushPromises()

    await openManualDialog(wrapper, series)
    await flushPromises()

    const seasonInput = findSeasonInput(wrapper)
    await seasonInput.setValue('2')
    await flushPromises()
    await findSearchButton(wrapper).trigger('click')
    await flushPromises()

    await findDispatchButton(wrapper).trigger('click')
    await flushPromises()

    expect(manualDispatchSubscription).toHaveBeenCalledWith('sub_series_dispatch', {
      candidate: expect.objectContaining({ id: 'c1' }),
      season: 2,
    })
  })

  it('整剧订阅切换季号后旧候选被清空，确认下发按钮被禁用', async () => {
    const series = buildSubscription({
      id: 'sub_series',
      type: 'TV',
      season: 0,
      name: 'Demo Show',
    })
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [series],
      total: 1,
    })
    vi.mocked(manualSearchSubscription).mockResolvedValue(
      resolveSearchResponse([
        {
          id: 'c1',
          title: 'Demo Show S01 1080p',
          site: 'Site A',
          size: 1024 * 1024 * 1024 * 5,
          seeders: 10,
          payload: { torrent_download: 'link' },
        },
      ]),
    )

    const wrapper = mountView()
    await flushPromises()

    await openManualDialog(wrapper, series)
    await flushPromises()

    // 整剧订阅打开弹窗不会自动搜索，需要管理员填季号
    const seasonInput = findSeasonInput(wrapper)
    expect(seasonInput.exists()).toBe(true)

    // 季号输入 1
    await seasonInput.setValue('1')
    await flushPromises()
    await findSearchButton(wrapper).trigger('click')
    await flushPromises()

    // 候选已出现
    expect(manualSearchSubscription).toHaveBeenCalledWith('sub_series', { season: 1 })
    expect(wrapper.text()).toContain('Demo Show S01 1080p')

    // 确认下发按钮此时可用（无 disabled 属性）
    expect(findDispatchButton(wrapper).attributes('disabled')).toBeUndefined()

    // 改成季号 2 但不重新搜索
    await seasonInput.setValue('2')
    await flushPromises()

    // 旧候选被清空
    expect(wrapper.text()).not.toContain('Demo Show S01 1080p')
    // 下发按钮被禁用
    const disabledAttr = findDispatchButton(wrapper).attributes('disabled')
    expect(disabledAttr).toBeDefined()
  })

  it('整剧订阅搜索失败后候选列表清空，不残留上次搜索结果', async () => {
    const series = buildSubscription({
      id: 'sub_series_fail',
      type: 'TV',
      season: 0,
      name: 'Demo Show',
    })
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [series],
      total: 1,
    })

    // 第一次搜索成功
    vi.mocked(manualSearchSubscription).mockResolvedValueOnce(
      resolveSearchResponse([
        {
          id: 'c1',
          title: 'Demo Show S01 1080p',
          site: 'Site A',
          size: 1024 * 1024 * 1024 * 5,
          seeders: 10,
          payload: {},
        },
      ]),
    )
    // 第二次搜索抛异常（全局错误提示由 axios 拦截器处理）
    vi.mocked(manualSearchSubscription).mockRejectedValueOnce(new Error('boom'))

    const wrapper = mountView()
    await flushPromises()

    await openManualDialog(wrapper, series)
    await flushPromises()

    const seasonInput = findSeasonInput(wrapper)
    await seasonInput.setValue('1')
    await flushPromises()
    await findSearchButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Demo Show S01 1080p')

    // 再次搜索（同一季号也行，重点测失败后清空）
    await findSearchButton(wrapper).trigger('click')
    await flushPromises()

    // 候选列表被清空，空态显示"暂无候选"
    expect(wrapper.text()).not.toContain('Demo Show S01 1080p')
    expect(wrapper.text()).toContain('暂无候选')
  })

  it('重新提交命中自动通过时提示等待入库', async () => {
    const rejected = buildSubscription({
      id: 'sub_rejected',
      status: 'REJECTED',
    })
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [rejected],
      total: 1,
    })
    vi.mocked(resubmitSubscription).mockResolvedValue({
      success: true,
      subscriptionId: 'sub_reapproved',
      status: 'APPROVED',
      autoApproved: true,
    })

    const wrapper = mountView()
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      openResubmitDialog: (sub: any) => void
      submitResubmission: () => Promise<void>
      resubmitNote: string
    }
    vm.openResubmitDialog(rejected)
    vm.resubmitNote = '补充说明'
    await vm.submitResubmission()
    await flushPromises()

    expect(ElMessage.success).toHaveBeenCalledWith('订阅已自动通过，等待入库')
  })

  it('管理员视图展示自动通过和人工通过标签', async () => {
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [
        buildSubscription({
          id: 'sub_auto',
          reviewSource: 'AUTO_QUOTA',
          reviewedAt: '2026-07-09T08:00:00Z',
        }),
        buildSubscription({
          id: 'sub_manual',
          reviewSource: 'MANUAL',
          reviewedAt: '2026-07-09T09:00:00Z',
        }),
      ],
      total: 2,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('自动通过')
    expect(wrapper.text()).toContain('人工通过')
  })
})

describe('SubscriptionsView 分页与二次提交兜底', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    authStoreState.isAdmin = true
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [],
      total: 0,
    })
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm' as never)
  })

  it('切换 pageSize 时重置到第 1 页，避免越界空页', async () => {
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [buildSubscription({ id: 'sub_a', status: 'APPROVED' })],
      total: 60,
    })

    const wrapper = mountView()
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      queryParams: { page: number; pageSize: number }
      handleSizeChange: (size: number) => void
    }
    // 模拟用户先翻到第 2 页，再切换 pageSize
    vm.queryParams.page = 2
    await flushPromises()

    vm.handleSizeChange(40)
    await flushPromises()

    expect(vm.queryParams.pageSize).toBe(40)
    expect(vm.queryParams.page).toBe(1)
    // 切换后请求带的页码应为 1
    const lastCall = vi.mocked(getSubscriptions).mock.calls.at(-1)?.[0] as any
    expect(lastCall.page).toBe(1)
    expect(lastCall.pageSize).toBe(40)
  })

  it('二次提交命中 confirmationRequired 后再失败不会逃出 catch', async () => {
    const rejected = buildSubscription({
      id: 'sub_rejected_v2',
      status: 'REJECTED',
    })
    vi.mocked(getSubscriptions).mockResolvedValue({
      data: [rejected],
      total: 1,
    })
    // 第一次 submitResubmission 抛 confirmationRequired
    vi.mocked(resubmitSubscription).mockRejectedValueOnce({
      response: {
        data: {
          confirmationRequired: true,
          existingSummary: { message: 'already exists' },
        },
      },
    })
    // 弹窗里点取消 → requestResubmitExistingConfirmation reject
    vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce(new Error('cancel'))

    const wrapper = mountView()
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      openResubmitDialog: (sub: any) => void
      handleResubmit: () => Promise<void>
      resubmitNote: string
      resubmitting: boolean
    }
    vm.openResubmitDialog(rejected)
    vm.resubmitNote = '补充说明'

    // handleResubmit 内的二次确认被取消，但不应抛出未捕获 rejection
    await expect(vm.handleResubmit()).resolves.toBeUndefined()
    // resubmitting 标志位正常复位
    expect(vm.resubmitting).toBe(false)
    // 二次确认弹窗确实被触发
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
  })
})
