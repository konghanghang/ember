import { describe, expect, it } from 'vitest'

import { isConflictError, isMessageBoxCancel } from './api-error'

describe('isConflictError', () => {
  it('识别 axios 风格的 409 响应', () => {
    expect(isConflictError({ response: { status: 409 } })).toBe(true)
  })

  it('其他状态码与非 axios 形状一律返回 false', () => {
    expect(isConflictError({ response: { status: 500 } })).toBe(false)
    expect(isConflictError({ response: {} })).toBe(false)
    expect(isConflictError({ response: null })).toBe(false)
    expect(isConflictError(new Error('409'))).toBe(false)
    expect(isConflictError('conflict')).toBe(false)
    expect(isConflictError(null)).toBe(false)
    expect(isConflictError(undefined)).toBe(false)
  })
})

describe('isMessageBoxCancel', () => {
  it('识别 ElMessageBox 的取消与关闭', () => {
    expect(isMessageBoxCancel('cancel')).toBe(true)
    expect(isMessageBoxCancel('close')).toBe(true)
  })

  it('真实错误不被误判为取消', () => {
    expect(isMessageBoxCancel(new Error('cancel'))).toBe(false)
    expect(isMessageBoxCancel({ response: { status: 500 } })).toBe(false)
    expect(isMessageBoxCancel('')).toBe(false)
    expect(isMessageBoxCancel(null)).toBe(false)
    expect(isMessageBoxCancel(undefined)).toBe(false)
  })
})
