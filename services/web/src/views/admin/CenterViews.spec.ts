import { defineComponent, h, nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PaymentCenterView from './PaymentCenterView.vue'
import PlaybackCenterView from './PlaybackCenterView.vue'
import RedemptionCodesView from './RedemptionCodesView.vue'
import RedemptionHistoryView from './RedemptionHistoryView.vue'
import UserCenterView from './UserCenterView.vue'
import PlanGroupsView from './PlanGroupsView.vue'

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, string | string[] | undefined>,
}))
const routerPush = vi.hoisted(() => vi.fn())
const routerReplace = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: routerPush,
    replace: routerReplace,
  }),
}))

const pageHeaderStub = defineComponent({
  name: 'EmberPageHeaderCard',
  props: {
    title: { type: String, required: true },
  },
  setup(props, { slots }) {
    return () => h('section', {
      'data-test': 'center-header',
      'data-title': props.title,
    }, [
      h('div', { 'data-test': 'center-actions' }, slots.actions?.()),
      slots.default?.(),
    ])
  },
})

const segmentTabsStub = defineComponent({
  name: 'EmberSegmentTabs',
  props: ['tabs', 'modelValue'],
  setup() {
    return () => h('div', { 'data-test': 'center-tabs' })
  },
})

const headerCases = [
  [UserCenterView, '用户中心'],
  [PaymentCenterView, '计费中心'],
  [PlaybackCenterView, '播放中心'],
] as const

describe('管理端中心页', () => {
  it.each(headerCases)('%s 由中心页统一承载标题', (component, title) => {
    routeState.query = {}
    const wrapper = shallowMount(component, {
      global: {
        stubs: {
          EmberPageHeaderCard: pageHeaderStub,
          EmberSegmentTabs: segmentTabsStub,
          'el-icon': true,
        },
      },
    })

    const headers = wrapper.findAll('[data-test="center-header"]')
    expect(headers).toHaveLength(1)
    expect(headers[0].attributes('data-title')).toBe(title)
  })

  it('计费中心根据 tab 查询参数直接展示套餐分组', () => {
    routeState.query = { tab: 'groups', syncBatchId: 'batch_1' }
    const wrapper = shallowMount(PaymentCenterView, {
      global: {
        stubs: {
          EmberPageHeaderCard: pageHeaderStub,
          EmberSegmentTabs: segmentTabsStub,
          'el-icon': true,
        },
      },
    })

    expect(wrapper.findComponent(PlanGroupsView).exists()).toBe(true)
  })

  it('用户中心只承载用户管理，不重复提供套餐分组', () => {
    routeState.query = { tab: 'groups' }
    const wrapper = shallowMount(UserCenterView, {
      global: {
        stubs: {
          EmberPageHeaderCard: pageHeaderStub,
          EmberSegmentTabs: segmentTabsStub,
        },
      },
    })

    expect(wrapper.findComponent(PlanGroupsView).exists()).toBe(false)
    expect(wrapper.findComponent(segmentTabsStub).exists()).toBe(false)
  })

  it('计费中心切换套餐分组时只更新当前路由的 tab', async () => {
    routeState.query = {}
    routerReplace.mockClear()
    const wrapper = shallowMount(PaymentCenterView, {
      global: {
        stubs: {
          EmberPageHeaderCard: pageHeaderStub,
          EmberSegmentTabs: segmentTabsStub,
        },
      },
    })

    wrapper.findComponent(segmentTabsStub).vm.$emit('change', 'groups')
    await nextTick()

    expect(routerReplace).toHaveBeenCalledWith({ query: { tab: 'groups' } })
  })
})


describe('计费中心兑换管理', () => {
  it.each([
    ['codes', RedemptionCodesView],
    ['history', RedemptionHistoryView],
  ] as const)('%s 直接嵌入对应页面并保留计费中心唯一页头', (tab, component) => {
    routeState.query = { tab }
    routerReplace.mockClear()
    const wrapper = shallowMount(PaymentCenterView, {
      global: { stubs: { EmberPageHeaderCard: pageHeaderStub, EmberSegmentTabs: segmentTabsStub } },
    })
    expect(wrapper.findComponent(component).exists()).toBe(true)
    expect(wrapper.findComponent(component).props('embedded')).toBe(true)
    expect(wrapper.findAll('[data-test="center-header"]')).toHaveLength(1)
    expect(routerReplace).not.toHaveBeenCalled()
    const tabs = wrapper.findComponent(segmentTabsStub)
    expect(tabs.props('modelValue')).toBe(tab)
    expect(tabs.props('tabs').map((item: { label: string }) => item.label))
      .toEqual(['付费方案', '支付记录', '兑换码', '兑换记录', '套餐分组'])
  })

  it.each(['codes', 'history'])('切换到 %s 保留其他查询参数', async (tab) => {
    routeState.query = { tab: 'plans', source: 'bookmark' }
    routerReplace.mockClear()
    const wrapper = shallowMount(PaymentCenterView, {
      global: { stubs: { EmberPageHeaderCard: pageHeaderStub, EmberSegmentTabs: segmentTabsStub } },
    })
    wrapper.findComponent(segmentTabsStub).vm.$emit('change', tab)
    await nextTick()
    expect(routerReplace).toHaveBeenCalledWith({ query: { tab, source: 'bookmark' } })
  })
})


describe('计费中心非法分段', () => {
  it.each(['unknown', ['codes', 'history']])('非法 tab %s 回到付费方案并保留其他参数', (tab) => {
    routeState.query = { tab, source: 'bookmark' }
    routerReplace.mockClear()
    const wrapper = shallowMount(PaymentCenterView, {
      global: { stubs: { EmberPageHeaderCard: pageHeaderStub, EmberSegmentTabs: segmentTabsStub } },
    })
    expect(wrapper.findComponent(segmentTabsStub).props('modelValue')).toBe('plans')
    expect(routerReplace).toHaveBeenCalledWith({ query: { tab: 'plans', source: 'bookmark' } })
  })
})
