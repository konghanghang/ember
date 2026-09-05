import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import TopBar from './TopBar.vue'

const routeState = vi.hoisted(() => ({
  name: 'console-dashboard',
  query: {} as Record<string, string | string[] | undefined>,
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn() },
}))

vi.mock('@/store/auth', () => ({
  useAuthStore: () => ({
    isAdmin: true,
    logout: vi.fn(),
  }),
}))

vi.mock('@/store/user', () => ({
  useUserStore: () => ({
    profile: {
      username: 'admin',
      email: 'admin@example.com',
    },
    clearUserData: vi.fn(),
  }),
}))

const routeTitles = [
  ['console-users', '用户中心'],
  ['console-billing', '计费中心'],
  ['console-playback', '播放中心'],
  ['console-redemptions', '兑换中心'],
  ['console-media-gaps', '缺集管理'],
  ['console-p115-accounts', '115 账号'],
  ['console-p115', '115 网盘'],
] as const

describe('TopBar', () => {
  it.each(routeTitles)('路由 %s 展示统一页面标题', (routeName, title) => {
    routeState.name = routeName
    const wrapper = shallowMount(TopBar, {
      props: { collapsed: false },
      global: {
        stubs: {
          DefaultAvatar: true,
          'el-dropdown': true,
          'el-icon': true,
        },
      },
    })

    expect(wrapper.get('p.text-lg').text()).toBe(title)
  })
})
