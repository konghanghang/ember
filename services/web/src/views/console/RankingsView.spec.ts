import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ElCheckbox, { ElCheckboxGroup } from 'element-plus/es/components/checkbox/index'

import RankingsView from './RankingsView.vue'
import { getLatestRanking, getRankingHistory } from '@/api/console'
import { getRankingLibraryAllowlist, previewRanking, updateRankingLibraryAllowlist } from '@/api/admin'
import { ElMessage } from 'element-plus'

vi.mock('@/api/console', () => ({
  getLatestRanking: vi.fn(),
  getRankingHistory: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  previewRanking: vi.fn(),
  getRankingLibraryAllowlist: vi.fn(),
  updateRankingLibraryAllowlist: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
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

const pageHeaderStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', [
      slots.default?.(),
      slots.actions?.(),
    ])
  },
})

const EmberFormDialogStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
    title: {
      type: String,
      default: '',
    },
  },
  emits: ['update:modelValue'],
  setup(props, { slots }) {
    return () =>
      props.modelValue
        ? h('div', {
          'data-test': 'allowlist-dialog',
          'data-title': props.title,
        }, [
          h('div', { 'data-test': 'dialog-body' }, slots.default?.()),
          h('div', { 'data-test': 'dialog-footer' }, slots.footer?.()),
        ])
        : null
  },
})

function mountView() {
  return mount(RankingsView, {
    global: {
      stubs: {
        EmberPageHeaderCard: pageHeaderStub,
        EmberFormDialog: EmberFormDialogStub,
        EmberSegmentTabs: passthroughStub,
        'el-icon': passthroughStub,
        'el-skeleton': true,
        'el-empty': true,
        'el-date-picker': true,
        'el-checkbox-group': ElCheckboxGroup,
        'el-checkbox': ElCheckbox,
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

function emptyRankingResponse() {
  return {
    period: 'daily' as const,
    periodStart: '2026-06-30',
    periodEnd: '2026-06-30',
    cutoffAt: '20:00',
    movies: [],
    episodes: [],
  }
}

describe('RankingsView 媒体库 allowlist', () => {
  afterEach(() => vi.restoreAllMocks())

  beforeEach(() => {
    vi.clearAllMocks()
    authStoreState.isAdmin = true
    vi.mocked(getLatestRanking).mockResolvedValue(emptyRankingResponse())
    vi.mocked(getRankingHistory).mockResolvedValue(emptyRankingResponse())
    vi.mocked(previewRanking).mockResolvedValue(emptyRankingResponse())
    vi.mocked(getRankingLibraryAllowlist).mockResolvedValue({
      data: {
        allowAll: false,
        libraryIds: ['lib_movie'],
        libraries: [
          { id: 'lib_movie', name: '电影库', type: 'movies', itemCount: 10 },
          { id: 'lib_series', name: '剧集库', type: 'tvshows', itemCount: 20 },
        ],
      },
    })
    vi.mocked(updateRankingLibraryAllowlist).mockResolvedValue({
      data: {
        allowAll: false,
        libraryIds: ['lib_movie', 'lib_series'],
        libraries: [
          { id: 'lib_movie', name: '电影库', type: 'movies', itemCount: 10 },
          { id: 'lib_series', name: '剧集库', type: 'tvshows', itemCount: 20 },
        ],
      },
    })
  })

  it('显示 API 业务时区的实际生成时间，而不是周期结束时间', async () => {
    vi.mocked(getLatestRanking).mockResolvedValue({
      ...emptyRankingResponse(),
      periodStart: '2026-09-29',
      periodEnd: '2026-09-29',
      snapshotAt: '2026-09-29T20:03:00+08:00',
      cutoffAt: '00:00',
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('生成于 2026/09/29 20:03')
    expect(wrapper.text()).not.toContain('截至 00:00')
    wrapper.unmount()
  })

  it('管理员可以加载并保存排行榜媒体库范围', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(getRankingLibraryAllowlist).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('未读取媒体库范围')
    expect(wrapper.find('[data-test="allowlist-dialog"]').exists()).toBe(false)

    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()

    expect(getRankingLibraryAllowlist).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="allowlist-dialog"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('参与统计的媒体库')
    expect(wrapper.text()).toContain('当前按 1 个媒体库统计')

    await wrapper.find('input[value="lib_series"]').setValue(true)
    await flushPromises()

    const saveButton = wrapper
      .findAll('button')
      .find(item => item.text().includes('保存媒体库范围'))
    expect(saveButton, '未找到保存按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateRankingLibraryAllowlist).toHaveBeenCalledWith([])
    expect(ElMessage.success).toHaveBeenCalledWith('已恢复为全部媒体库参与统计')
    expect(wrapper.find('[data-test="allowlist-dialog"]').exists()).toBe(false)
  })

  it('媒体库使用显式选中值，取消与保存不触发弃用警告', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('input[value="lib_movie"]').element).toHaveProperty('checked', true)
    await wrapper.find('input[value="lib_series"]').setValue(true)
    await wrapper.find('input[value="lib_movie"]').setValue(false)
    const save = wrapper.findAll('button').find(button => button.text().includes('保存媒体库范围'))!
    await save.trigger('click')
    await flushPromises()

    expect(updateRankingLibraryAllowlist).toHaveBeenCalledWith(['lib_series'])
    expect(warn.mock.calls.flat().map(String).join('\n')).not.toContain('label act as value')
    wrapper.unmount()
  })

  it('普通用户不显示媒体库配置区块', async () => {
    authStoreState.isAdmin = false
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('参与统计的媒体库')
    expect(wrapper.find('[data-test="open-allowlist-dialog"]').exists()).toBe(false)
    expect(getRankingLibraryAllowlist).not.toHaveBeenCalled()
  })

  it('存在失效媒体库时显示提示', async () => {
    vi.mocked(getRankingLibraryAllowlist).mockResolvedValueOnce({
      data: {
        allowAll: false,
        libraryIds: [],
        invalidLibraryIds: ['lib_missing'],
        libraries: [
          { id: 'lib_movie', name: '电影库', type: 'movies', itemCount: 10 },
        ],
      },
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('已失效媒体库')
    expect(wrapper.text()).toContain('当前配置仅包含失效媒体库')
    expect(wrapper.text()).not.toContain('当前按全部媒体库统计')

    const resetButton = wrapper
      .findAll('button')
      .find(item => item.text().includes('恢复全库统计'))
    expect(resetButton, '未找到恢复按钮').toBeTruthy()
    expect((resetButton!.element as HTMLButtonElement).disabled).toBe(false)
  })

  it('预览态保存后会自动重新预览', async () => {
    const wrapper = mountView()
    await flushPromises()

    const previewButton = wrapper
      .findAll('button')
      .find(item => item.text().includes('预览生成'))
    expect(previewButton, '未找到预览按钮').toBeTruthy()
    await previewButton!.trigger('click')
    await flushPromises()

    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()

    await wrapper.find('input[value="lib_series"]').setValue(true)
    await flushPromises()

    const saveButton = wrapper
      .findAll('button')
      .find(item => item.text().includes('保存媒体库范围'))
    expect(saveButton, '未找到保存按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(previewRanking).toHaveBeenCalledTimes(2)
  })

  it('恢复全库统计失败时保留原有选择状态', async () => {
    vi.mocked(updateRankingLibraryAllowlist).mockRejectedValueOnce(new Error('boom'))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()

    const resetButton = wrapper
      .findAll('button')
      .find(item => item.text().includes('恢复全库统计'))
    expect(resetButton, '未找到恢复按钮').toBeTruthy()
    await resetButton!.trigger('click')
    await flushPromises()

    // 共享决策：catch 内的 ElMessage.error 已删除（错误文案由 request 拦截器统一弹出），
    // 这里只验证本地状态被回滚到原有选择。
    expect(wrapper.text()).toContain('当前按 1 个媒体库统计')
    expect(wrapper.find('input[value="lib_movie"]').element).toHaveProperty('checked', true)
  })

  it('读取失败明确显示错误、禁止保存，并可重试恢复选择列表', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(getRankingLibraryAllowlist).mockRejectedValueOnce(new Error('fixture unavailable'))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('媒体库列表读取失败')
    expect(wrapper.text()).not.toContain('当前没有可选媒体库')
    const save = wrapper.findAll('button').find(button => button.text().includes('保存媒体库范围'))!
    expect(save.element).toHaveProperty('disabled', true)
    await save.trigger('click')
    expect(updateRankingLibraryAllowlist).not.toHaveBeenCalled()

    await wrapper.find('[data-test="retry-allowlist"]').trigger('click')
    await flushPromises()
    expect(getRankingLibraryAllowlist).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('媒体库列表读取失败')
    expect(wrapper.find('input[value="lib_movie"]').exists()).toBe(true)
    expect(wrapper.find('input[value="lib_series"]').exists()).toBe(true)
    expect(save.element).toHaveProperty('disabled', false)
    wrapper.unmount()
  })

  it('成功返回空列表时显示空状态，并可重新读取', async () => {
    vi.mocked(getRankingLibraryAllowlist).mockResolvedValueOnce({
      data: { allowAll: true, libraryIds: [], libraries: [] },
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-test="open-allowlist-dialog"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('当前没有可选媒体库')
    expect(wrapper.text()).not.toContain('媒体库列表读取失败')
    await wrapper.find('[data-test="retry-allowlist"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('input[value="lib_movie"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('管理员首屏不会主动请求媒体库配置', async () => {
    mountView()
    await flushPromises()

    expect(getLatestRanking).toHaveBeenCalledTimes(1)
    expect(getRankingLibraryAllowlist).not.toHaveBeenCalled()
  })
})
