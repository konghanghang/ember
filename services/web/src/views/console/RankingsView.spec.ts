import { defineComponent, h, inject, provide, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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

const checkboxGroupKey = Symbol('checkbox-group')

const CheckboxGroupStub = defineComponent({
  props: {
    modelValue: {
      type: Array as () => string[],
      default: () => [],
    },
  },
  emits: ['update:modelValue'],
  setup(props, { emit, slots }) {
    provide(checkboxGroupKey, {
      isChecked: (label: string) => props.modelValue.includes(label),
      toggle: (label: string, checked: boolean) => {
        const next = new Set(props.modelValue)
        if (checked) {
          next.add(label)
        } else {
          next.delete(label)
        }
        emit('update:modelValue', Array.from(next))
      },
    })
    return () => h('div', slots.default?.())
  },
})

const CheckboxStub = defineComponent({
  props: {
    label: {
      type: String,
      required: true,
    },
  },
  setup(props) {
    const group = inject<{
      isChecked: (label: string) => boolean
      toggle: (label: string, checked: boolean) => void
    }>(checkboxGroupKey)

    return () =>
      h('input', {
        type: 'checkbox',
        checked: group?.isChecked(props.label) ?? false,
        'data-test': `library-checkbox-${props.label}`,
        onChange: (event: Event) => {
          group?.toggle(props.label, (event.target as HTMLInputElement).checked)
        },
      })
  },
})

function mountView() {
  return mount(RankingsView, {
    global: {
      stubs: {
        EmberPageHeaderCard: pageHeaderStub,
        EmberFormDialog: EmberFormDialogStub,
        EmberSegmentTabs: passthroughStub,
        EmberEmptyStateCard: passthroughStub,
        'el-icon': passthroughStub,
        'el-skeleton': true,
        'el-empty': true,
        'el-date-picker': true,
        'el-checkbox-group': CheckboxGroupStub,
        'el-checkbox': CheckboxStub,
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

    await wrapper.find('[data-test="library-checkbox-lib_series"]').setValue(true)
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

    await wrapper.find('[data-test="library-checkbox-lib_series"]').setValue(true)
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
    expect(wrapper.find('[data-test="library-checkbox-lib_movie"]').element).toHaveProperty('checked', true)
  })

  it('管理员首屏不会主动请求媒体库配置', async () => {
    mountView()
    await flushPromises()

    expect(getLatestRanking).toHaveBeenCalledTimes(1)
    expect(getRankingLibraryAllowlist).not.toHaveBeenCalled()
  })
})
