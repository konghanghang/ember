import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createP115Account,
  getP115Account,
  getP115Accounts,
  replaceP115AccountCookie,
  setP115AccountEnabled,
  updateP115AccountPlaybackConfig,
  updateP115AccountSourceLocation,
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
      cookie: 'UID=100_A1_1700000000',
      userAgent: 'Ember Test',
      targetParentId: 'target-1',
    })
    await replaceP115AccountCookie('account/1', {
      cookie: 'UID=200_A2_1700000000',
      appType: 'custom_client',
    })
    await updateP115AccountSourceLocation('account/1', {
      embyPathPrefix: '/mnt/cloudNAS/115lifetime',
      sourceRootId: '0',
    })
    await updateP115AccountPlaybackConfig('account/1', {
      targetParentPath: '/Ember/Playback',
      maxConcurrentStreams: 3,
    })
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
          cookie: 'UID=100_A1_1700000000',
          userAgent: 'Ember Test',
          targetParentId: 'target-1',
        },
      }],
      [{
        url: '/admin/p115-accounts/account%2F1/cookie',
        method: 'put',
        data: {
          cookie: 'UID=200_A2_1700000000',
          appType: 'custom_client',
        },
      }],
      [{
        url: '/admin/p115-accounts/account%2F1/source-location',
        method: 'put',
        data: {
          embyPathPrefix: '/mnt/cloudNAS/115lifetime',
          sourceRootId: '0',
        },
      }],
      [{
        url: '/admin/p115-accounts/account%2F1/playback-config',
        method: 'put',
        data: {
          targetParentPath: '/Ember/Playback',
          maxConcurrentStreams: 3,
        },
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
