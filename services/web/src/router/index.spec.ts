import { describe, expect, it } from 'vitest'

import router from './index'

describe('console 115 account routes', () => {
  it('主路由受管理员角色保护，旧管理路径只做重定向', () => {
    const mainRoute = router.getRoutes().find(route => route.name === 'console-p115-accounts')
    expect(mainRoute?.path).toBe('/console/p115-accounts')
    expect(mainRoute?.meta.role).toBe('admin')

    const legacyRoute = router.getRoutes().find(route => route.path === '/admin/p115-accounts')
    expect(legacyRoute?.redirect).toBe('/console/p115-accounts')
  })

  it('个人 115 网盘使用独立用户控制台路由', () => {
    const personalRoute = router.getRoutes().find(route => route.name === 'console-p115')
    expect(personalRoute?.path).toBe('/console/p115')
    expect(personalRoute?.meta.role).toBe('user')
  })
})

describe('plan group page routes', () => {
  it.each(['/console/plan-groups', '/admin/plan-groups'])('%s 不再注册独立页面或兼容重定向', (path) => {
    expect(router.getRoutes().find(route => route.path === path)).toBeUndefined()
  })
})
