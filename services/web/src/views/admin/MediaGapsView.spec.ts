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
  dispatchMediaGap,
  deleteMediaGap,
  batchDeleteMediaGaps,
  getGroupedMediaGaps,
  getMediaGapScanStatus,
  getMediaGaps,
  scanMediaGaps,
  searchMediaGap,
} from '@/api/admin'

vi.mock('@/api/admin', () => ({
  dispatchMediaGap: vi.fn(),
  deleteMediaGap: vi.fn(),
  batchDeleteMediaGaps: vi.fn(),
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
    return () => h('div', [slots.default?.(), slots.actions?.(), slots.header?.(), slots.titleSuffix?.(), slots.pagination?.()])
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
        EmberEmptyStateCard: defineComponent({
          props: { title: String, description: String },
          setup(props, { slots }) {
            return () => h('div', [props.title, props.description, slots.actions?.()])
          },
        }),
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
        // 表格列的 scoped slot 由真实 el-table 提供 row，空表 stub 不调用它。
        'el-table-column': defineComponent({ setup: () => () => h('div') }),
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
    vi.mocked(deleteMediaGap).mockReset()
    vi.mocked(batchDeleteMediaGaps).mockReset()
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

  it.each(['search', 'dispatch'])('%s 状态冲突关闭候选并刷新列表，禁止旧候选再次下发', async (operation) => {
    const gap = buildGap()
    const candidate = { id: 'candidate-1', title: 'Demo', payload: { url: 'fake' } }
    vi.mocked(searchMediaGap).mockResolvedValue({ data: { mediaGap: gap, candidates: [candidate] } })
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as {
      openSearchDialog: (gap: MediaGapItem) => Promise<void>
      handleDialogSearch: () => Promise<void>
      handleDispatch: () => Promise<void>
      currentGap: MediaGapItem | null
      candidateResult: MediaGapSearchResult
      selectedCandidateId: string
    }
    await vm.openSearchDialog(gap)
    expect(wrapper.find('[data-test="form-dialog"]').exists()).toBe(true)
    const previousFetches = vi.mocked(getGroupedMediaGaps).mock.calls.length
    const conflict = { isAxiosError: true, response: { status: 409 } }
    if (operation === 'search') {
      vi.mocked(searchMediaGap).mockRejectedValueOnce(conflict)
      await vm.handleDialogSearch()
    } else {
      vi.mocked(dispatchMediaGap).mockRejectedValueOnce(conflict)
      await vm.handleDispatch()
    }
    await resolvePending()
    expect(wrapper.find('[data-test="form-dialog"]').exists()).toBe(false)
    expect(vm.currentGap).toBeNull()
    expect(vm.candidateResult.candidates).toEqual([])
    expect(vm.selectedCandidateId).toBe('')
    expect(getGroupedMediaGaps).toHaveBeenCalledTimes(previousFetches + 1)
    expect(ElMessage.error).not.toHaveBeenCalled()
    const previousDispatches = vi.mocked(dispatchMediaGap).mock.calls.length
    await vm.handleDispatch()
    expect(dispatchMediaGap).toHaveBeenCalledTimes(previousDispatches)
    wrapper.unmount()
  })

  it('默认查询未收口工单，下发普通失败也刷新列表并保留候选', async () => {
    const gap = buildGap()
    const candidate = { id: 'candidate-1', title: 'Demo', payload: { url: 'fake' } }
    vi.mocked(searchMediaGap).mockResolvedValue({ data: { mediaGap: gap, candidates: [candidate] } })
    const wrapper = mountView()
    await resolvePending()
    expect(getGroupedMediaGaps).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'OPEN' }))
    const vm = wrapper.vm as unknown as {
      openSearchDialog: (gap: MediaGapItem) => Promise<void>
      handleDispatch: () => Promise<void>
      candidateResult: MediaGapSearchResult
    }
    await vm.openSearchDialog(gap)
    const before = vi.mocked(getGroupedMediaGaps).mock.calls.length
    vi.mocked(dispatchMediaGap).mockRejectedValueOnce({ isAxiosError: true, response: { status: 500 } })
    await vm.handleDispatch()
    expect(getGroupedMediaGaps).toHaveBeenCalledTimes(before + 1)
    expect(vm.candidateResult.candidates).toEqual([candidate])
    wrapper.unmount()
  })

  it('历史筛选保留已入库季集，收口后采用后端回退页码', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as {
      queryParams: { status: string; page: number }
      fetchData: () => Promise<void>
      displayedSeasonGroups: (series: { seasons: Array<{ season: number; gaps: MediaGapItem[] }> }) => Array<{ gaps: MediaGapItem[] }>
    }
    const season = { seasons: [{ season: 1, gaps: [buildGap({ status: 'INGESTED' }), buildGap({ id: 'ignored', status: 'IGNORED' })] }] }
    expect(vm.displayedSeasonGroups(season)[0].gaps).toHaveLength(2)
    vm.queryParams.status = 'ALL'
    expect(vm.displayedSeasonGroups(season)[0].gaps).toHaveLength(2)
    vm.queryParams.page = 3
    vi.mocked(getGroupedMediaGaps).mockResolvedValueOnce({ ...emptyGroupedResponse(), page: 1 })
    await vm.fetchData()
    expect(getGroupedMediaGaps).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'ALL', page: 3 }))
    expect(vm.queryParams.page).toBe(1)
    wrapper.unmount()
  })

  it('已下发工单须确认重发并携带当前版本，取消不发送', async () => {
    const gap = buildGap({ status: 'REQUESTED' })
    const candidate = { id: 'candidate-1', title: 'Demo', payload: { url: 'fake' } }
    vi.mocked(searchMediaGap).mockResolvedValue({ data: { mediaGap: gap, candidates: [candidate] } })
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as { openSearchDialog: (gap: MediaGapItem) => Promise<void>; handleDispatch: () => Promise<void> }
    await vm.openSearchDialog(gap)
    vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce('cancel')
    await vm.handleDispatch()
    expect(dispatchMediaGap).not.toHaveBeenCalled()
    vi.mocked(ElMessageBox.confirm).mockResolvedValueOnce('confirm' as never)
    vi.mocked(dispatchMediaGap).mockResolvedValueOnce({ data: { mediaGap: gap } })
    await vm.handleDispatch()
    expect(dispatchMediaGap).toHaveBeenCalledWith(gap.id, expect.objectContaining({ retry: true, expectedUpdatedAt: gap.updatedAt }))
    wrapper.unmount()
  })

  it('扫描部分完成显示失败项，支持只重试该剧', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as { scanStatus: MediaGapScanStatus; refreshScanStatus: (notify: boolean) => Promise<void> }
    vm.scanStatus = { scanId: 'scan-1', status: 'running', running: true }
    vi.mocked(getMediaGapScanStatus).mockResolvedValueOnce({ data: {
      scanId: 'scan-1', status: 'partial', running: false, message: '扫描部分完成',
      failures: [{ tmdbId: '123', seriesName: '失败剧集', season: 2, reason: '季元数据获取失败' }]
    } })
    const before = vi.mocked(getGroupedMediaGaps).mock.calls.length
    await vm.refreshScanStatus(true)
    await resolvePending()
    expect(ElMessage.warning).toHaveBeenCalledWith('扫描部分完成')
    expect(getGroupedMediaGaps).toHaveBeenCalledTimes(before + 1)
    expect(wrapper.text()).toContain('失败剧集 S2：季元数据获取失败')
    vi.mocked(ElMessageBox.confirm).mockResolvedValueOnce('confirm' as never)
    vi.mocked(scanMediaGaps).mockResolvedValueOnce({ data: { scanId: 'retry-1', running: true, status: 'running' } })
    await wrapper.findAll('button').find((button) => button.text() === '重试该剧')!.trigger('click')
    await resolvePending()
    expect(scanMediaGaps).toHaveBeenCalledWith({ tmdbId: '123', force: true })
    wrapper.unmount()
  })

  it('未收口工单不能删除，取消确认不发送请求', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as { handleDeleteGaps: (gaps: MediaGapItem[]) => Promise<void> }
    for (const status of ['MISSING', 'SEARCHED', 'REQUESTED', 'DISPATCH_FAILED'] as const) {
      await vm.handleDeleteGaps([buildGap({ status })])
    }
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce('cancel')
    await vm.handleDeleteGaps([buildGap({ status: 'IGNORED' })])
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(expect.stringContaining('扫描可能重新生成'), '删除工单', expect.anything())
    expect(deleteMediaGap).not.toHaveBeenCalled()
    expect(batchDeleteMediaGaps).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([1, 2])('删除 %i 条已收口记录后刷新，不提供清空全部', async (count) => {
    const gaps = [buildGap({ status: 'INGESTED' }), buildGap({ id: 'gap_2', status: 'IGNORED' })].slice(0, count)
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as { handleDeleteGaps: (gaps: MediaGapItem[]) => Promise<void> }
    vi.mocked(deleteMediaGap).mockResolvedValueOnce({ data: { deletedCount: 1 } })
    vi.mocked(batchDeleteMediaGaps).mockResolvedValueOnce({ data: { deletedCount: count } })
    const before = vi.mocked(getGroupedMediaGaps).mock.calls.length
    await vm.handleDeleteGaps(gaps)
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(expect.stringContaining(`永久删除 ${count} 条`), '删除工单', expect.anything())
    if (count === 1) expect(deleteMediaGap).toHaveBeenCalledWith('gap_1')
    else expect(batchDeleteMediaGaps).toHaveBeenCalledWith(['gap_1', 'gap_2'])
    expect(getGroupedMediaGaps).toHaveBeenCalledTimes(before + 1)
    expect(ElMessage.success).toHaveBeenCalledWith(`已删除 ${count} 条工单`)
    wrapper.unmount()
  })

  it('删除状态冲突会刷新，明细页超出新总数时回退并清除选择', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as {
      viewMode: string; queryParams: { page: number; pageSize: number }
      selectedDeleteGaps: MediaGapItem[]
      handleDeleteGaps: (gaps: MediaGapItem[]) => Promise<void>
    }
    vm.viewMode = 'table'
    await resolvePending()
    vm.queryParams.page = 2
    vm.queryParams.pageSize = 20
    vm.selectedDeleteGaps = [buildGap({ status: 'IGNORED' })]
    vi.mocked(deleteMediaGap).mockRejectedValueOnce({ isAxiosError: true, response: { status: 409 } })
    vi.mocked(getMediaGaps).mockResolvedValue({ data: [], total: 0, page: 1, pageSize: 20 })
    await vm.handleDeleteGaps(vm.selectedDeleteGaps)
    expect(vm.queryParams.page).toBe(1)
    expect(vm.selectedDeleteGaps).toEqual([])
    expect(ElMessage.success).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('勾选仅保留当前页终态记录，重复点击确认期间不重复发起删除', async () => {
    const wrapper = mountView()
    await resolvePending()
    const closed = buildGap({ status: 'IGNORED' })
    const open = buildGap({ id: 'open', status: 'REQUESTED' })
    const vm = wrapper.vm as unknown as {
      tableData: MediaGapItem[]; selectedDeleteGaps: MediaGapItem[]
      canSelectForDelete: (gap: MediaGapItem) => boolean
      handleDeleteSelection: (gaps: MediaGapItem[]) => void
      handleDeleteGaps: (gaps: MediaGapItem[]) => Promise<void>
    }
    vm.tableData = [closed, open]
    expect(vm.canSelectForDelete(closed)).toBe(true)
    expect(vm.canSelectForDelete(open)).toBe(false)
    vm.handleDeleteSelection([closed, open, buildGap({ id: 'other-page', status: 'INGESTED' })])
    expect(vm.selectedDeleteGaps).toEqual([closed])
    let confirm!: () => void
    vi.mocked(ElMessageBox.confirm).mockImplementationOnce(() => new Promise((resolve) => { confirm = () => resolve('confirm' as never) }))
    vi.mocked(deleteMediaGap).mockResolvedValueOnce({ data: { deletedCount: 1 } })
    const pending = vm.handleDeleteGaps([closed])
    await vm.handleDeleteGaps([closed])
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    expect(deleteMediaGap).not.toHaveBeenCalled()
    confirm()
    await pending
    expect(deleteMediaGap).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('点击查询失败时不把旧历史卡片显示成已收口结果', async () => {
    const wrapper = mountView()
    await resolvePending()
    const gap = buildGap({ status: 'INGESTED', seriesName: '旧历史剧集' })
    const series: MediaGapGroupedResponse['data'][number] = {
      key: 'old', seriesName: gap.seriesName, gaps: [gap], seasons: [{ season: 1, gaps: [gap] }],
      totalGaps: 1, missingCount: 0, searchedCount: 0, requestedCount: 0, ingestedCount: 1, ignoredCount: 0
    }
    const vm = wrapper.vm as unknown as {
      queryParams: { status: string }
      fetchData: () => Promise<void>
      loadError: string
      groupedData: MediaGapGroupedResponse['data']
    }
    vm.queryParams.status = 'ALL'
    vi.mocked(getGroupedMediaGaps).mockResolvedValueOnce({ ...emptyGroupedResponse(), data: [series], total: 1, itemTotal: 1 })
    await vm.fetchData()
    await resolvePending()
    expect(wrapper.text()).toContain('旧历史剧集')
    vm.queryParams.status = 'OPEN'
    vi.mocked(getGroupedMediaGaps).mockRejectedValueOnce({ isAxiosError: true, response: { status: 400 } })
    await vm.fetchData()
    await resolvePending()
    expect(vm.loadError).toContain('查询失败')
    expect(vm.loadError).toContain('400')
    expect(vm.groupedData).toEqual([])
    expect(wrapper.findAll('.series-card')).toHaveLength(0)
    vi.mocked(getGroupedMediaGaps).mockResolvedValueOnce(emptyGroupedResponse())
    await vm.fetchData()
    expect(vm.loadError).toBe('')
    wrapper.unmount()
  })

  it('尚未提交的状态选项不能隐藏上次成功查询返回的历史季集', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as {
      queryParams: { status: string }
      displayedSeasonGroups: (series: { seasons: Array<{ season: number; gaps: MediaGapItem[] }> }) => Array<{ gaps: MediaGapItem[] }>
    }
    const series = { seasons: [{ season: 1, gaps: [buildGap({ status: 'IGNORED' })] }] }
    vm.queryParams.status = 'ALL'
    expect(vm.displayedSeasonGroups(series)[0].gaps).toHaveLength(1)
    vm.queryParams.status = 'OPEN'
    expect(vm.displayedSeasonGroups(series)[0]?.gaps).toHaveLength(1)
    wrapper.unmount()
  })

  it('旧查询的迟到失败不能覆盖较新的成功结果', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as { fetchData: () => Promise<void>; loadError: string; total: number }
    let rejectOld!: (error: Error) => void
    vi.mocked(getGroupedMediaGaps).mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
    const oldRequest = vm.fetchData()
    vi.mocked(getGroupedMediaGaps).mockResolvedValueOnce({ ...emptyGroupedResponse(), total: 2 })
    await vm.fetchData()
    rejectOld(new Error('late failure'))
    await oldRequest
    expect(vm.loadError).toBe('')
    expect(vm.total).toBe(2)
    wrapper.unmount()
  })

  it('成功响应返回期间切换筛选，不把返回的历史工单误标为已收口空卡或缺集', async () => {
    const wrapper = mountView()
    await resolvePending()
    const vm = wrapper.vm as unknown as {
      queryParams: { status: string }
      fetchData: () => Promise<void>
      groupedData: MediaGapGroupedResponse['data']
    }
    const gap = buildGap({ status: 'INGESTED', seriesName: '历史剧集' })
    const response: MediaGapGroupedResponse = {
      ...emptyGroupedResponse(), total: 1, itemTotal: 1,
      data: [{ key: 'history', seriesName: gap.seriesName, gaps: [gap], seasons: [{ season: 1, gaps: [gap] }],
        totalGaps: 1, missingCount: 0, searchedCount: 0, requestedCount: 0, ingestedCount: 1, ignoredCount: 0 }]
    }
    let finish!: (response: MediaGapGroupedResponse) => void
    vi.mocked(getGroupedMediaGaps).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    vm.queryParams.status = 'ALL'
    const pending = vm.fetchData()
    expect(getGroupedMediaGaps).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'ALL' }))
    vm.queryParams.status = 'OPEN'
    finish(response)
    await pending
    await resolvePending()
    expect(wrapper.findAll('.series-card')).toHaveLength(1)
    expect(wrapper.findAll('.episode-chip')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('已收口到已忽略或已入库摘要')
    expect(wrapper.text()).not.toContain('缺 1 集')
    expect(wrapper.text()).not.toContain('忽略本季缺集')
    wrapper.unmount()
  })

})
