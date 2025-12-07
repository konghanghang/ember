'use server'

import { cookies } from 'next/headers'
import { redirect } from 'next/navigation'
import { prisma } from '@/lib/db'
import { signToken, verifyPassword, getServerAuth } from '@/lib/auth'

/**
 * 管理员登录
 * @param data 登录信息 { username, password }
 * @returns { success: boolean, error?: string } 或直接 redirect
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
    const token = await signToken({
      id: admin.id,
      username: admin.username,
    })

    // 4. 设置 httpOnly cookie（安全存储）
    const cookieStore = await cookies()
    cookieStore.set('auth-token', token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 7, // 7天
      path: '/',
    })

    // 5. 记录登录日志
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

    // 6. 重定向到管理后台（这会抛出 NEXT_REDIRECT，属于正常流程）
    redirect('/admin/invites')
  } catch (error) {
    // redirect() 会抛出 NEXT_REDIRECT 错误，需要重新抛出
    if (error && typeof error === 'object' && 'digest' in error) {
      throw error
    }

    console.error('管理员登录失败：', error)
    return {
      success: false,
      error: '登录失败，请稍后重试',
    }
  }
}

/**
 * 获取当前登录用户信息
 * @returns 用户信息或 null
 */
export async function getCurrentUser() {
  const auth = await getServerAuth()
  return auth
}

/**
 * 管理员登出
 * 清除 cookie 并重定向到登录页
 */
export async function adminLogout() {
  const cookieStore = await cookies()
  cookieStore.delete('auth-token')
  redirect('/login')
}
