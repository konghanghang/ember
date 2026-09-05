import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import EmberPageHeaderCard from './EmberPageHeaderCard.vue'

describe('EmberPageHeaderCard', () => {
  it('完整页面模式展示标题、说明、统计和操作区', () => {
    const wrapper = mount(EmberPageHeaderCard, {
      props: {
        title: '用户管理',
        description: '管理用户账号',
      },
      slots: {
        titleSuffix: '<span data-test="summary">共 3 个用户</span>',
        actions: '<button data-test="action">新建用户</button>',
        default: '<div data-test="content">页面内容</div>',
      },
    })

    expect(wrapper.get('h1').text()).toBe('用户管理共 3 个用户')
    expect(wrapper.text()).toContain('管理用户账号')
    expect(wrapper.get('[data-test="action"]').text()).toBe('新建用户')
    expect(wrapper.get('[data-test="content"]').text()).toBe('页面内容')
  })

  it('嵌入模式隐藏重复标题和说明，但保留统计、操作与内容', () => {
    const wrapper = mount(EmberPageHeaderCard, {
      props: {
        title: '用户管理',
        description: '管理用户账号',
        hideTitle: true,
      },
      slots: {
        titleSuffix: '<span data-test="summary">共 3 个用户</span>',
        actions: '<button data-test="action">新建用户</button>',
        default: '<div data-test="content">页面内容</div>',
      },
    })

    expect(wrapper.find('h1').exists()).toBe(false)
    expect(wrapper.get('section').attributes('aria-label')).toBe('用户管理')
    expect(wrapper.text()).not.toContain('管理用户账号')
    expect(wrapper.get('[data-test="summary"]').text()).toBe('共 3 个用户')
    expect(wrapper.get('[data-test="action"]').text()).toBe('新建用户')
    expect(wrapper.get('[data-test="content"]').text()).toBe('页面内容')
  })
})
