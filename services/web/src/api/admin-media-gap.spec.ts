import { describe, expect, it, vi } from 'vitest'
import { deleteMediaGap, batchDeleteMediaGaps } from './admin'
import { request } from './request'

vi.mock('./request', () => ({ request: vi.fn().mockResolvedValue({ data: { deletedCount: 1 } }) }))

describe('缺集删除接口', () => {
  it('单条编码路径，批量只发送显式 ID 列表', async () => {
    await deleteMediaGap('gap/1')
    await batchDeleteMediaGaps(['gap1', 'gap2'])
    expect(vi.mocked(request).mock.calls).toEqual([
      [{ url: '/admin/media-gaps/gap%2F1', method: 'delete' }],
      [{ url: '/admin/media-gaps/batch-delete', method: 'post', data: { ids: ['gap1', 'gap2'] } }],
    ])
  })
})
