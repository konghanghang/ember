/**
 * 判断请求是否被后端同步闸门拦截（HTTP 409），
 * 调用方据此把冲突显式转成"同步中/稍后重试"的专门提示。
 */
export function isConflictError(error: unknown): boolean {
  return typeof error === 'object'
    && error !== null
    && 'response' in error
    && (error as { response?: { status?: number } }).response?.status === 409
}

/**
 * 判断 ElMessageBox 的 reject 是否来自用户取消/关闭。
 * ElMessageBox 取消时 reject 字符串 'cancel'，点关闭时 reject 'close'，
 * 这两类应被静默吞掉，不能当成失败提示。
 */
export function isMessageBoxCancel(error: unknown): boolean {
  return error === 'cancel' || error === 'close'
}
