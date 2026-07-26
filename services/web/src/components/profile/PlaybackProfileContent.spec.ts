import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PlaybackProfileContent from './PlaybackProfileContent.vue'
import type { UserPlaybackProfile } from '@/types/api'

// Ember 基础组件在测试里替换为透传 stub，避免依赖 Element Plus 全局注册。
// EmberPageHeaderCard 必须把 #actions / #titleSuffix / 默认 slot 全部渲染出来，
// 否则后代 EmberSegmentTabs / EmberDateRangeField 不会进入 DOM，无法触发事件。
const pageHeaderStub = defineComponent({
  props: {
    title: { type: String, required: true },
  },
  setup(props, { slots }) {
    return () =>
      h('div', { 'data-test': 'page-header' }, [
        h('h1', { 'data-test': 'page-title' }, props.title),
        slots.titleSuffix?.(),
        slots.actions?.(),
        slots.default?.(),
      ])
  },
})

const metricCardStub = defineComponent({
  props: {
    title: { type: String, required: true },
    value: { type: [String, Number], required: true },
    detail: { type: String, default: '' },
    tone: { type: String, default: 'neutral' },
  },
  setup(props) {
    return () =>
      h('div', { 'data-test': 'metric-card' }, [
        h('p', { 'data-test': 'metric-title' }, props.title),
        h('p', { 'data-test': 'metric-value' }, String(props.value)),
        props.detail ? h('p', { 'data-test': 'metric-detail' }, props.detail) : null,
      ])
  },
})

const segmentTabsStub = defineComponent({
  props: {
    modelValue: { type: String, required: true },
    tabs: { type: Array as () => Array<{ key: string; label: string }>, required: true },
    ariaLabel: { type: String, required: true },
  },
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    return () =>
      h('div', { 'data-test': 'segment-tabs', 'aria-label': props.ariaLabel }, [
        h(
          'button',
          {
            'data-test': 'segment-tab-first',
            onClick: () => emit('change', props.tabs[0]?.key ?? ''),
          },
          '切换到首个'
        ),
        h(
          'button',
          {
            'data-test': 'segment-tab-unknown',
            onClick: () => emit('change', 'not-a-range'),
          },
          '非法值'
        ),
      ])
  },
})

const dateRangeFieldStub = defineComponent({
  props: {
    modelValue: { type: Array as unknown as () => [string, string] | null, default: null },
    label: { type: String, required: true },
  },
  emits: ['update:modelValue', 'change', 'calendar-change'],
  setup(props, { emit }) {
    return () =>
      h('div', { 'data-test': 'date-range-field' }, [
        h('p', { 'data-test': 'date-range-label' }, props.label),
        h(
          'button',
          {
            'data-test': 'date-range-clear',
            onClick: () => emit('change', null),
          },
          '清空'
        ),
        h(
          'button',
          {
            'data-test': 'date-range-apply',
            onClick: () => emit('change', ['2026-01-01 00:00:00', '2026-01-31 23:59:59']),
          },
          '应用'
        ),
        h(
          'button',
          {
            'data-test': 'date-range-calendar',
            onClick: () => emit('calendar-change', [new Date('2026-01-15T00:00:00')]),
          },
          '日历选择'
        ),
      ])
  },
})

const emptyStateStub = defineComponent({
  props: {
    title: { type: String, required: true },
    compact: { type: Boolean, default: false },
  },
  setup(props) {
    return () => h('div', { 'data-test': 'empty-state' }, props.title)
  },
})

function buildProfile(overrides: Partial<UserPlaybackProfile> = {}): UserPlaybackProfile {
  return {
    userId: 'u1',
    username: 'alice',
    range: 'today',
    totalPlayCount: 10,
    totalPlayDuration: 600,
    totalPlayDurationFormatted: '10m',
    activeDays: 3,
    averagePlayDuration: 60,
    averagePlayDurationFormatted: '1m',
    lastPlayedAt: '2026-01-01 10:00:00',
    hourlyDistribution: [],
    deviceDistribution: [],
    clientDistribution: [],
    badges: [],
    recentRecords: [],
    ...overrides,
  }
}

function mountContent(props: Record<string, unknown> = {}) {
  return mount(PlaybackProfileContent, {
    props: {
      title: '我的画像',
      profile: buildProfile(),
      loading: false,
      rangeOptions: [
        { label: '当天', value: 'today' },
        { label: '近 7 天', value: '7d' },
      ],
      selectedRange: 'today',
      selectedRangeLabel: '当天',
      customDateRange: null,
      disabledCustomDate: () => false,
      ...props,
    },
    global: {
      stubs: {
        EmberPageHeaderCard: pageHeaderStub,
        EmberMetricCard: metricCardStub,
        EmberSegmentTabs: segmentTabsStub,
        EmberDateRangeField: dateRangeFieldStub,
        EmberEmptyStateCard: emptyStateStub,
        'el-tooltip': true,
      },
    },
  })
}

describe('PlaybackProfileContent', () => {
  it('渲染头卡标题与选中的时间窗口徽标', () => {
    const wrapper = mountContent({ title: 'Alice 的画像' })

    expect(wrapper.find('[data-test="page-title"]').text()).toBe('Alice 的画像')
    expect(wrapper.text()).toContain('当天')
  })

  it('EmberSegmentTabs 切换合法范围时会发出 range-change', async () => {
    const wrapper = mountContent()

    await wrapper.find('[data-test="segment-tab-first"]').trigger('click')

    expect(wrapper.emitted('range-change')).toEqual([['today']])
  })

  it('EmberSegmentTabs 收到非法范围时不会发出 range-change', async () => {
    const wrapper = mountContent()

    await wrapper.find('[data-test="segment-tab-unknown"]').trigger('click')

    expect(wrapper.emitted('range-change')).toBeUndefined()
  })

  it('EmberDateRangeField 应用范围时发出 custom-range-change', async () => {
    const wrapper = mountContent()

    await wrapper.find('[data-test="date-range-apply"]').trigger('click')

    // custom-range-change 由 @change 监听直接透传，是调用方实际依赖的契约。
    expect(wrapper.emitted('custom-range-change')).toEqual([
      [['2026-01-01 00:00:00', '2026-01-31 23:59:59']],
    ])
  })

  it('EmberDateRangeField 清空时把空值透传给 custom-range-change', async () => {
    const wrapper = mountContent()

    await wrapper.find('[data-test="date-range-clear"]').trigger('click')

    expect(wrapper.emitted('custom-range-change')).toEqual([[null]])
  })

  it('EmberDateRangeField 的 calendar-change 透传给 custom-calendar-change', async () => {
    const wrapper = mountContent()

    await wrapper.find('[data-test="date-range-calendar"]').trigger('click')

    const events = wrapper.emitted('custom-calendar-change')
    expect(events).toHaveLength(1)
    const payload = (events![0] as unknown[])[0] as unknown[]
    expect(payload[0]).toBeInstanceOf(Date)
  })

  it('分段控件 ariaLabel 提供业务语义', () => {
    const wrapper = mountContent()

    expect(wrapper.find('[data-test="segment-tabs"]').attributes('aria-label')).toBe(
      '画像时间窗口切换'
    )
  })

  it('统计卡数量与 profile 字段一致', () => {
    const wrapper = mountContent({
      profile: buildProfile({ totalPlayCount: 42, activeDays: 7 }),
    })

    const titles = wrapper.findAll('[data-test="metric-title"]').map(node => node.text())
    expect(titles).toEqual(['累计播放时长', '播放次数', '活跃天数', '最近播放'])
    const values = wrapper.findAll('[data-test="metric-value"]').map(node => node.text())
    expect(values).toContain('42')
    expect(values).toContain('7')
  })

  it('活跃天数与最近播放统计卡不再渲染已被 §2.2.1 删除的解释文案', () => {
    const wrapper = mountContent()

    const details = wrapper.findAll('[data-test="metric-detail"]').map(node => node.text())
    // 累计播放时长 / 播放次数 仍保留可执行 detail（时间窗口、平均单次），其余两张卡不再写解释。
    expect(details).not.toContain('有播放的天数越多，节奏越稳定')
    expect(details).not.toContain('只展示最近一次播放')
  })

  it('画像标签不向用户展示内部 ID', () => {
    const wrapper = mountContent({
      profile: buildProfile({
        badges: [
          {
            id: 'badge-internal-001',
            name: '夜猫子',
            description: '常在凌晨播放',
          },
        ],
      }),
    })

    expect(wrapper.text()).toContain('夜猫子')
    expect(wrapper.text()).not.toContain('badge-internal-001')
    expect(wrapper.text()).not.toContain('标签 ID')
    // 内部 ID 不得通过任何 DOM 通道暴露给用户：包括 title 属性（hover 原生提示）、
    // data-*、aria-* 等。html() 覆盖文本与全部属性，比仅查 text() 更严。
    expect(wrapper.html()).not.toContain('badge-internal-001')
    expect(wrapper.findAll('[title]').map(node => node.attributes('title'))).toEqual([])
  })
})

// 显式抑制 vitest 对未使用导入的告警（保留 h 供 stub 使用）。
void vi
