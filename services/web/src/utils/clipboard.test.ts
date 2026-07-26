import { afterEach, describe, expect, it, vi } from 'vitest'

import { copyToClipboard } from './clipboard'

function stubClipboardApi(impl?: () => Promise<void>) {
  const writeText = impl ? vi.fn(impl) : vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(globalThis.navigator, 'clipboard', {
    value: { writeText },
    configurable: true
  })
  return writeText
}

function stubExecCommand(result: boolean) {
  const execCommand = vi.fn().mockReturnValue(result)
  Object.defineProperty(document, 'execCommand', {
    value: execCommand,
    configurable: true
  })
  return execCommand
}

afterEach(() => {
  Reflect.deleteProperty(globalThis.navigator, 'clipboard')
  Reflect.deleteProperty(document, 'execCommand')
})

describe('copyToClipboard', () => {
  it('clipboard API 可用时直接写入并返回成功', async () => {
    const writeText = stubClipboardApi()
    await expect(copyToClipboard('hello')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
  })

  it('clipboard API 拒绝时降级 execCommand', async () => {
    stubClipboardApi(() => Promise.reject(new Error('denied')))
    const execCommand = stubExecCommand(true)

    await expect(copyToClipboard('fallback')).resolves.toBe(true)
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('clipboard API 不存在（非安全上下文）时走降级路径', async () => {
    const execCommand = stubExecCommand(true)
    await expect(copyToClipboard('legacy')).resolves.toBe(true)
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('两条路径都失败时返回 false', async () => {
    stubClipboardApi(() => Promise.reject(new Error('denied')))
    stubExecCommand(false)
    await expect(copyToClipboard('nope')).resolves.toBe(false)
  })

  it('降级路径的临时 textarea 用完即移除', async () => {
    Reflect.deleteProperty(globalThis.navigator, 'clipboard')
    stubExecCommand(true)

    await copyToClipboard('cleanup')
    expect(document.querySelector('textarea')).toBeNull()
  })
})
