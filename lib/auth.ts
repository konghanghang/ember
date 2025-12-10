import 'server-only'
import bcrypt from 'bcryptjs'
import { SignJWT, jwtVerify } from 'jose'
import { cookies } from 'next/headers'

// JWT 密钥（从环境变量读取）
const JWT_SECRET = process.env.JWT_SECRET || 'default-secret-key-change-this-in-production'

// JWT Token 有效期：7 天（秒）
const JWT_EXPIRES_IN = 7 * 24 * 60 * 60 // 7天

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
export async function signToken(payload: JwtPayload): Promise<string> {
  const secret = new TextEncoder().encode(JWT_SECRET)
  const token = await new SignJWT({ ...payload })
    .setProtectedHeader({ alg: 'HS256' })
    .setExpirationTime(Math.floor(Date.now() / 1000) + JWT_EXPIRES_IN)
    .setIssuedAt()
    .sign(secret)
  return token
}

/**
 * 验证 JWT Token
 * @throws {Error} Token 无效或过期时抛出异常
 */
export async function verifyToken(token: string): Promise<JwtPayload> {
  try {
    const secret = new TextEncoder().encode(JWT_SECRET)
    const { payload } = await jwtVerify(token, secret)

    // 从 payload 中提取我们的自定义字段
    if (!payload.id || !payload.username) {
      throw new Error('Token payload 格式无效')
    }

    return {
      id: payload.id as string,
      username: payload.username as string,
    }
  } catch (error) {
    if (error instanceof Error) {
      if (error.message.includes('expired')) {
        throw new Error('Token 已过期')
      }
    }
    throw new Error('Token 无效')
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

/**
 * 服务端获取当前认证用户信息
 * 从 httpOnly cookie 中读取并验证 token
 * @returns 用户信息或 null（未登录/token无效）
 */
export async function getServerAuth(): Promise<JwtPayload | null> {
  try {
    const cookieStore = await cookies()
    const token = cookieStore.get('auth-token')?.value

    if (!token) {
      return null
    }

    const payload = verifyToken(token)
    return payload
  } catch {
    // Token 无效或过期
    return null
  }
}
