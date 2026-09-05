import { defineComponent, h, nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PaymentCenterView from './PaymentCenterView.vue'
import PlaybackCenterView from './PlaybackCenterView.vue'
import RedemptionCenterView from './RedemptionCenterView.vue'
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
  setup() {
    return () => h('div', { 'data-test': 'center-tabs' })
  },
})

const headerCases = [
  [UserCenterView, '用户中心'],
  [PaymentCenterView, '计费中心'],
  [PlaybackCenterView, '播放中心'],
  [RedemptionCenterView, '兑换中心'],
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
