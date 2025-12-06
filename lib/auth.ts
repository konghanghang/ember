import bcrypt from 'bcryptjs'
import jwt from 'jsonwebtoken'

// JWT 密钥（从环境变量读取）
const JWT_SECRET = process.env.JWT_SECRET || 'default-secret-key-change-this-in-production'

// JWT Token 有效期：7 天
const JWT_EXPIRES_IN = '7d'

/**
 * JWT Token 载荷接口
 */
export interface JwtPayload {
  id: string
  username: string
}

/**
 * 生成 JWT Token
 */
export function signToken(payload: JwtPayload): string {
  return jwt.sign(payload, JWT_SECRET, {
    expiresIn: JWT_EXPIRES_IN,
  })
}

/**
 * 验证 JWT Token
 * @throws {Error} Token 无效或过期时抛出异常
 */
export function verifyToken(token: string): JwtPayload {
  try {
    const decoded = jwt.verify(token, JWT_SECRET) as JwtPayload
    return decoded
  } catch (error) {
    if (error instanceof jwt.TokenExpiredError) {
      throw new Error('Token 已过期')
    }
    if (error instanceof jwt.JsonWebTokenError) {
      throw new Error('Token 无效')
    }
    throw new Error('Token 验证失败')
  }
}

/**
 * 加密密码
 * @param password 明文密码
 * @returns bcrypt hash (cost=10)
 */
export async function hashPassword(password: string): Promise<string> {
  return bcrypt.hash(password, 10)
}

/**
 * 验证密码
 * @param password 明文密码
 * @param hash 加密后的密码
 * @returns 密码是否匹配
 */
export async function verifyPassword(password: string, hash: string): Promise<boolean> {
  return bcrypt.compare(password, hash)
}
