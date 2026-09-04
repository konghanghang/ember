import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createPersonalP115Account,
  getPersonalP115Account,
  getPersonalP115Usage,
  replacePersonalP115Cookie,
  revokePersonalP115Account,
  setPersonalP115Enabled,
  updatePersonalP115Concurrency,
  updatePersonalP115Directory,
  validatePersonalP115Account,
} from './console'
import { request } from './request'

vi.mock('./request', () => ({
  request: vi.fn(),
}))

describe('personal 115 account API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(request).mockResolvedValue({} as never)
  })

  it('所有操作只使用当前认证用户路由', async () => {
    await getPersonalP115Account()
    await getPersonalP115Usage()
    await createPersonalP115Account('UID=100_F1_1700000000')
    await replacePersonalP115Cookie('UID=200_Z9_1700000000')
    await updatePersonalP115Directory('/Ember/Playback')
    await updatePersonalP115Concurrency(3)
    await validatePersonalP115Account()
    await setPersonalP115Enabled(true)
    await revokePersonalP115Account()

    expect(vi.mocked(request).mock.calls).toEqual([
      [{ url: '/user/p115-account', method: 'get', silent: true }],
      [{ url: '/user/p115-usage', method: 'get', silent: true }],
      [{ url: '/user/p115-account', method: 'post', data: { cookie: 'UID=100_F1_1700000000' } }],
      [{ url: '/user/p115-account/cookie', method: 'put', data: { cookie: 'UID=200_Z9_1700000000' } }],
      [{ url: '/user/p115-account/directory', method: 'put', data: { targetParentPath: '/Ember/Playback' } }],
      [{ url: '/user/p115-account/concurrency', method: 'put', data: { maxConcurrentStreams: 3 } }],
      [{ url: '/user/p115-account/validate', method: 'post' }],
      [{ url: '/user/p115-account/enabled', method: 'put', data: { enabled: true } }],
      [{ url: '/user/p115-account', method: 'delete' }],
    ])
  })
})
