'use server'

import { prisma } from '@/lib/db'
import { signToken, verifyPassword } from '@/lib/auth'

/**
 * 管理员登录
 * @param data 登录信息 { username, password }
 * @returns { success: boolean, token?: string, error?: string }
 */
export async function adminLogin(data: { username: string; password: string }) {
  try {
    // 1. 查找管理员
    const admin = await prisma.admin.findUnique({
      where: { username: data.username },
    })

    // 2. 验证管理员存在性和密码
    if (!admin || !(await verifyPassword(data.password, admin.password))) {
      return {
        success: false,
        error: '用户名或密码错误',
      }
    }

    // 3. 生成 JWT Token
    const token = signToken({
      id: admin.id,
      username: admin.username,
    })

    // 4. 记录登录日志
    await prisma.log.create({
      data: {
        action: 'admin_login',
        targetId: admin.id,
        details: {
          username: admin.username,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
      token,
    }
  } catch (error) {
    console.error('管理员登录失败：', error)
    return {
      success: false,
      error: '登录失败，请稍后重试',
    }
  }
}
