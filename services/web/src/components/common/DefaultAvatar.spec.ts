import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import DefaultAvatar from './DefaultAvatar.vue'

describe('DefaultAvatar', () => {
  it('keeps a fixed avatar footprint in flex layouts', () => {
    const wrapper = mount(DefaultAvatar, {
      props: {
        name: 'alice',
        size: 'md',
        shape: 'full',
      },
    })

    expect(wrapper.classes()).toContain('shrink-0')
    expect(wrapper.classes()).toContain('h-10')
    expect(wrapper.classes()).toContain('w-10')
    expect(wrapper.classes()).toContain('rounded-full')
  })
})
