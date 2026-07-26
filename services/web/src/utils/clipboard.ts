/**
 * 复制文本到剪贴板。
 * 优先走 navigator.clipboard（需安全上下文），失败或不可用时降级 execCommand，
 * 覆盖非 HTTPS 内网部署与旧浏览器。返回是否复制成功，成功/失败提示由调用方决定。
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    // clipboard API 不存在（非安全上下文）或被拒绝时走降级路径
  }
  return legacyCopyToClipboard(text)
}

/** execCommand('copy') 降级路径；jsdom 等无 execCommand 环境下直接返回 false。 */
function legacyCopyToClipboard(text: string): boolean {
  if (typeof document === 'undefined' || typeof document.execCommand !== 'function') {
    return false
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  // 移出视口避免闪烁；不能用 display:none，否则无法选中
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  document.body.appendChild(textarea)
  textarea.select()

  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    document.body.removeChild(textarea)
  }
}
