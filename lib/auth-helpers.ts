/**
 * 认证相关的工具函数
 *
 * 职责：
 * 1. 纯函数：密码验证逻辑
 * 2. 工具函数：密码加密
 */

// ==================== 纯函数：密码验证 ====================

/**
 * 验证密码强度
 * @param password 密码
 * @returns { valid: boolean, reason?: string }
 */
export function validatePasswordStrength(password: string): {
  valid: boolean
  reason?: string
} {
  if (password.length < 6) {
    return { valid: false, reason: '密码长度至少 6 个字符' }
  }

  return { valid: true }
}

/**
 * 验证密码是否符合复杂度要求（可选）
 *
 * 当前只检查长度，未来可以扩展：
 * - 必须包含数字
 * - 必须包含大小写字母
 * - 必须包含特殊字符
 *
 * @param password 密码
 * @returns 是否符合要求
 */
export function isPasswordComplex(password: string): boolean {
  // 当前只检查长度
  return password.length >= 6
}

// ==================== 工具函数：密码加密 ====================

/**
 * 加密密码（使用 bcrypt）
 * @param password 明文密码
 * @param rounds bcrypt 轮次（默认 10）
 * @returns 加密后的密码
 */
export async function hashPassword(
  password: string,
  rounds: number = 10
): Promise<string> {
  const bcrypt = await import('bcryptjs')
  return bcrypt.hash(password, rounds)
}
