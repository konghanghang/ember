import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createP115Account,
  getP115Account,
  getP115Accounts,
  replaceP115AccountCookie,
  setP115AccountEnabled,
  validateP115Account,
} from './admin'
import { request } from './request'

vi.mock('./request', () => ({
  request: vi.fn(),
}))

describe('admin 115 account API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(request).mockResolvedValue({} as never)
  })

  it('使用独立账号管理路由，并保持 Cookie 只出现在写请求中', async () => {
    await getP115Accounts()
    await getP115Account('account/1')
    await createP115Account({
      role: 'playback',
      alias: '播放小号',
      cookie: 'UID=100_A1',
      appType: 'web',
      userAgent: 'Ember Test',
      targetParentId: 'target-1',
    })
    await replaceP115AccountCookie('account/1', 'UID=200_A1')
    await validateP115Account('account/1')
    await setP115AccountEnabled('account/1', true)

    expect(vi.mocked(request).mock.calls).toEqual([
      [{ url: '/admin/p115-accounts', method: 'get' }],
      [{ url: '/admin/p115-accounts/account%2F1', method: 'get' }],
      [{
        url: '/admin/p115-accounts',
        method: 'post',
        data: {
          role: 'playback',
          alias: '播放小号',
          cookie: 'UID=100_A1',
          appType: 'web',
          userAgent: 'Ember Test',
          targetParentId: 'target-1',
        },
      }],
      [{
        url: '/admin/p115-accounts/account%2F1/cookie',
        method: 'put',
        data: { cookie: 'UID=200_A1' },
      }],
      [{ url: '/admin/p115-accounts/account%2F1/validate', method: 'post' }],
      [{
        url: '/admin/p115-accounts/account%2F1/enabled',
        method: 'put',
        data: { enabled: true },
      }],
    ])
  })
})
