import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

import MediaGapsView from './MediaGapsView.vue'
import type {
  MediaGapGroupedResponse,
  MediaGapItem,
  MediaGapListResponse,
  MediaGapScanResponse,
  MediaGapScanStatus,
  MediaGapSearchResult
} from '@/types/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getGroupedMediaGaps,
  getMediaGapScanStatus,
  getMediaGaps,
  scanMediaGaps,
  searchMediaGap,
} from '@/api/admin'

vi.mock('@/api/admin', () => ({
  dispatchMediaGap: vi.fn(),
  getMediaGapScanStatus: vi.fn(),
  getMediaGaps: vi.fn(),
  getGroupedMediaGaps: vi.fn(),
  ignoreMediaGap: vi.fn(),
  scanMediaGaps: vi.fn(),
  searchMediaGap: vi.fn(),
}))

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

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', [slots.default?.(), slots.actions?.(), slots.titleSuffix?.(), slots.pagination?.()])
  },
})

const trueStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

function mountView() {
  return mount(MediaGapsView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        EmberPageHeaderCard: passthroughStub,
        EmberFilterPanel: passthroughStub,
        EmberTableCard: passthroughStub,
        EmberEmptyStateCard: passthroughStub,
        EmberFormDialog: defineComponent({
          props: {
            modelValue: { type: Boolean, default: false },
            title: { type: String, default: '' },
            width: { type: String, default: '' },
          },
          emits: ['update:modelValue'],
          setup(props, { slots }) {
            return () =>
              props.modelValue
                ? h(
                    'div',
                    {
                      'data-test': 'form-dialog',
                      'data-title': props.title,
                      'data-width': props.width,
                    },
                    [
                      h('div', { 'data-test': 'form-dialog-body' }, slots.default?.()),
                      h('div', { 'data-test': 'form-dialog-footer' }, slots.footer?.()),
                    ],
                  )
                : null
          },
        }),
        EmberSegmentTabs: defineComponent({
          props: {
            modelValue: { type: String, default: '' },
            tabs: { type: Array, default: () => [] },
            ariaLabel: { type: String, default: '' },
          },
          emits: ['update:modelValue', 'change'],
          setup(props) {
            return () =>
              h(
                'div',
                {
                  'data-test': 'segment-tabs',
                  'aria-label': props.ariaLabel,
                  'data-value': props.modelValue,
                },
                (props.tabs as Array<{ key: string; label: string }>)
                  .map((tab) => `${tab.key}:${tab.label}`)
                  .join('|'),
              )
          },
        }),
        EmberSearchInput: trueStub,
        EmberSelectField: trueStub,
        EmberDateRangeField: trueStub,
        'el-icon': passthroughStub,
        'el-tag': passthroughStub,
        'el-pagination': trueStub,
        'el-table-column': trueStub,
        'el-option': trueStub,
        Calendar: passthroughStub,
        CircleCheck: passthroughStub,
        CircleCheckFilled: passthroughStub,
        CircleClose: passthroughStub,
        Clock: passthroughStub,
        Collection: passthroughStub,
        Download: passthroughStub,
        Grid: passthroughStub,
        InfoFilled: passthroughStub,
        Loading: passthroughStub,
        RefreshRight: passthroughStub,
        Remove: passthroughStub,
        Search: passthroughStub,
        Upload: passthroughStub,
        Warning: passthroughStub,
      },
    },
  })
}

function buildGap(overrides: Partial<MediaGapItem> = {}): MediaGapItem {
  return {
    id: 'gap_1',
    seriesName: 'Demo Series',
    season: 1,
    episode: 1,
    status: 'MISSING',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-02T00:00:00Z',
    ...overrides,
  }
}

function emptyGroupedResponse(): MediaGapGroupedResponse {
  return {
    data: [],
    total: 0,
    itemTotal: 0,
    page: 1,
    pageSize: 9,
    summary: {
      missingCount: 0,
      searchedCount: 0,
      requestedCount: 0,
      ingestedCount: 0,
      ignoredCount: 0,
    },
  }
}

function emptyListResponse(): MediaGapListResponse {
  return { data: [], total: 0, page: 1, pageSize: 20 }
}

function idleScanStatus(): MediaGapScanStatus {
  return { status: 'idle', running: false, message: '暂无扫描任务' }
}

async function resolvePending(): Promise<void> {
  // 让轮询定时器、flushPromises 一并消化，避免 onMounted 的 refreshScanStatus 残留 pending。
  await flushPromises()
  await flushPromises()
}

describe('MediaGapsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.mocked(getGroupedMediaGaps).mockResolvedValue(emptyGroupedResponse())
    vi.mocked(getMediaGaps).mockResolvedValue(emptyListResponse())
    vi.mocked(getMediaGapScanStatus).mockResolvedValue({ data: idleScanStatus() })
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm' as never)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('P1-2: 扫描接口抛错后按钮恢复可点（scanStatus.running 单一事实源驱动 disabled）', async () => {
    vi.mocked(scanMediaGaps).mockRejectedValueOnce(new Error('boom'))

    const wrapper = mountView()
    await resolvePending()

    const findScanButton = () =>
      wrapper
        .findAll('button')
        .find((btn) => btn.text().includes('触发全库扫描') || btn.text().includes('扫描中'))

    // 触发扫描：确认框通过，scanMediaGaps 抛错
    await findScanButton()!.trigger('click')
    await flushPromises()

    // 错误由 request 拦截器统一弹窗，本视图不重复弹
    expect(ElMessage.error).not.toHaveBeenCalled()
    // scanStatus 仍为 idle（接口未把 running 置 true），扫描按钮恢复可点
    const scanButton = findScanButton()
    expect(scanButton).toBeTruthy()
    expect(scanButton!.attributes('disabled')).toBeUndefined()
    expect(scanButton!.text()).toContain('触发全库扫描')
    expect(scanButton!.text()).not.toContain('扫描中')
  })

  it('P1-2: 扫描启动后 scanStatus.running=true 时按钮置灰，文案切到"扫描中"', async () => {
    const started: MediaGapScanResponse = {
      async: true,
      running: true,
      status: 'running',
      message: '缺集扫描已启动',
    }
    vi.mocked(scanMediaGaps).mockResolvedValueOnce({ data: started })

    const wrapper = mountView()
    await resolvePending()

    const findScanButton = () =>
      wrapper
        .findAll('button')
        .find((btn) => btn.text().includes('触发全库扫描') || btn.text().includes('扫描中'))

    await findScanButton()!.trigger('click')
    await flushPromises()

    const scanButton = findScanButton()
    expect(scanButton).toBeTruthy()
    expect(scanButton!.attributes('disabled')).toBeDefined()
    expect(scanButton!.text()).toContain('扫描中')
  })

  it('P2-7: fetchData 请求乱序时后发结果生效，先发请求的迟到响应被丢弃', async () => {
    // 构造两组 grouped 响应：第一次慢、第二次快
    const slowGap = buildGap({ id: 'gap_slow', seriesName: 'Slow Series' })
    const fastGap = buildGap({ id: 'gap_fast', seriesName: 'Fast Series' })

    const slowResponse: MediaGapGroupedResponse = {
      ...emptyGroupedResponse(),
      data: [
        {
          key: 'slow',
          seriesName: 'Slow Series',
          gaps: [slowGap],
          seasons: [{ season: 1, gaps: [slowGap] }],
          totalGaps: 1,
          missingCount: 1,
          searchedCount: 0,
          requestedCount: 0,
          ingestedCount: 0,
          ignoredCount: 0,
        },
      ],
      total: 1,
      itemTotal: 1,
    }
    const fastResponse: MediaGapGroupedResponse = {
      ...emptyGroupedResponse(),
      data: [
        {
          key: 'fast',
          seriesName: 'Fast Series',
          gaps: [fastGap],
          seasons: [{ season: 1, gaps: [fastGap] }],
          totalGaps: 1,
          missingCount: 1,
          searchedCount: 0,
          requestedCount: 0,
          ingestedCount: 0,
          ignoredCount: 0,
        },
      ],
      total: 1,
      itemTotal: 1,
    }

    let callCount = 0
    vi.mocked(getGroupedMediaGaps).mockImplementation(() => {
      callCount += 1
      // 第一次（慢）和第二次（快），保证乱序
      if (callCount === 1) {
        return new Promise((resolve) =>
          setTimeout(() => resolve(slowResponse), 50),
        )
      }
      return Promise.resolve(fastResponse)
    })

    const wrapper = mountView()
    await flushPromises()

    // 主动触发第二次 fetchData（在第一次未完成时立即发起）
    const vm = wrapper.vm as unknown as { fetchData: () => Promise<void> }
    const firstCall = vm.fetchData()
    const secondCall = vm.fetchData()
    await Promise.all([firstCall, secondCall])
    await resolvePending()

    // 慢响应应被令牌守卫丢弃，渲染的应是快响应的剧名
    expect(wrapper.text()).toContain('Fast Series')
    expect(wrapper.text()).not.toContain('Slow Series')
  })

  it('视图切换走 EmberSegmentTabs（不再手写按钮组），ariaLabel 提供业务语义', async () => {
    const wrapper = mountView()
    await resolvePending()

    const tabs = wrapper.findAll('[data-test="segment-tabs"]')
    // 至少存在视图切换分段；排序分段在 grouped 模式下也存在
    const viewTabs = tabs.find((node) => node.attributes('aria-label') === '缺集视图切换')
    expect(viewTabs, '缺集视图切换分段未渲染').toBeTruthy()
    expect(viewTabs!.attributes('data-value')).toBe('grouped')
    expect(viewTabs!.text()).toContain('grouped:聚合视图')
    expect(viewTabs!.text()).toContain('table:明细视图')
  })

  it('搜索弹窗走 EmberFormDialog（统一 chrome），宽度收敛到 680px 基线', async () => {
    const gap = buildGap({ id: 'gap_search', status: 'MISSING' })
    vi.mocked(getGroupedMediaGaps).mockResolvedValueOnce({
      ...emptyGroupedResponse(),
      data: [
        {
          key: 'k',
          seriesName: 'Demo Series',
          gaps: [gap],
          seasons: [{ season: 1, gaps: [gap] }],
          totalGaps: 1,
          missingCount: 1,
          searchedCount: 0,
          requestedCount: 0,
          ingestedCount: 0,
          ignoredCount: 0,
        },
      ],
      total: 1,
      itemTotal: 1,
    })

    const searchResult: MediaGapSearchResult = { candidates: [] }
    vi.mocked(searchMediaGap).mockResolvedValue({ data: searchResult })

    const wrapper = mountView()
    await resolvePending()

    // 打开搜索弹窗：点击"搜索当前集"
    const openSearch = wrapper
      .findAll('button')
      .find((btn) => btn.text().includes('搜索当前集'))
    expect(openSearch, '未找到搜索当前集按钮').toBeTruthy()
    await openSearch!.trigger('click')
    await flushPromises()

    const dialog = wrapper.find('[data-test="form-dialog"]')
    expect(dialog.exists(), '搜索弹窗未走 EmberFormDialog').toBe(true)
    expect(dialog.attributes('data-width')).toBe('680px')
    expect(dialog.attributes('data-title')).toBe('搜索候选并下发')
  })
})
