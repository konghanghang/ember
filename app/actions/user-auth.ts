'use server'

import { cookies } from 'next/headers'
import { redirect } from 'next/navigation'
import { prisma } from '@/lib/db'
import { signToken, getServerAuth } from '@/lib/auth'
import { embyClient } from '@/lib/emby'

/**
 * 用户登录
 * @param data 登录信息 { username, password }
 * @returns { success: boolean, error?: string } 或直接 redirect
 */
export async function userLogin(data: { username: string; password: string }) {
  try {
    // 1. 从数据库查找用户
    const user = await prisma.user.findUnique({
      where: { username: data.username },
    })

    if (!user) {
      return {
        success: false,
        error: '用户名或密码错误',
      }
    }

    // 2. 检查用户状态
    if (!user.isActive) {
      return {
        success: false,
        error: '账号已被禁用，请联系管理员',
      }
    }

    // 3. 检查是否过期
    if (user.expiresAt && new Date(user.expiresAt) < new Date()) {
      return {
        success: false,
        error: '账号已过期，请联系管理员续期',
      }
    }

    // 4. 使用 Emby API 验证密码
    try {
      await embyClient.authenticateUser(data.username, data.password)
    } catch (error) {
      console.error('Emby 认证失败：', error)
      return {
        success: false,
        error: '用户名或密码错误',
      }
    }

    // 5. 生成 JWT Token（role: 'user'）
    const token = await signToken({
      id: user.id,
      username: user.username,
      role: 'user',
    })

    // 6. 设置 httpOnly cookie
    const cookieStore = await cookies()
    cookieStore.set('auth-token', token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 7, // 7天
      path: '/',
    })

    // 7. 记录登录日志
    await prisma.log.create({
      data: {
        action: 'user_login',
        targetId: user.id,
        details: {
          username: user.username,
          timestamp: new Date().toISOString(),
        },
      },
    })

    // 8. 重定向到用户仪表盘
    redirect('/user/dashboard')
  } catch (error) {
    // redirect() 会抛出 NEXT_REDIRECT 错误，需要重新抛出
    if (error && typeof error === 'object' && 'digest' in error) {
      throw error
    }

    console.error('用户登录失败：', error)
    return {
      success: false,
      error: '登录失败，请稍后重试',
    }
  }
}

/**
 * 获取当前登录的用户信息（含数据库详细信息）
 * @returns 用户信息或 null
 */
export async function getUserAuth() {
  const auth = await getServerAuth()

  // 只返回 role 为 'user' 的认证信息
  if (!auth || auth.role !== 'user') {
    return null
  }

  // 从数据库获取完整用户信息
  const user = await prisma.user.findUnique({
    where: { id: auth.id },
    include: {
      invite: true, // 包含邀请码信息
    },
  })

  if (!user) {
    return null
  }

  return {
    id: user.id,
    username: user.username,
    email: user.email,
    embyId: user.embyId,
    isActive: user.isActive,
    expiresAt: user.expiresAt,
    createdAt: user.createdAt,
    inviteCode: user.invite?.code,
  }
}

/**
 * 用户登出
 * 清除 cookie 并重定向到登录页
 */
export async function userLogout() {
  const cookieStore = await cookies()
  cookieStore.delete('auth-token')
  redirect('/user/login')
}

/**
 * 修改用户密码
 * @param data { currentPassword, newPassword }
 * @returns { success: boolean, error?: string }
 */
export async function updateUserPassword(data: {
  currentPassword: string
  newPassword: string
}) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'user') {
      return {
        success: false,
        error: '未登录或登录已过期',
      }
    }

    // 2. 从数据库获取用户信息
    const user = await prisma.user.findUnique({
      where: { id: auth.id },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 3. 使用 Emby API 验证当前密码
    try {
      await embyClient.authenticateUser(user.username, data.currentPassword)
    } catch (error) {
      console.error('Emby 认证失败：', error)
      return {
        success: false,
        error: '当前密码错误',
      }
    }

    // 4. 同步到 Emby（设置新密码）
    await embyClient.setUserPassword(user.embyId, data.newPassword)

    // 5. 记录日志
    await prisma.log.create({
      data: {
        action: 'user_update_password',
        targetId: user.id,
        details: {
          username: user.username,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('修改密码失败：', error)
    return {
      success: false,
      error: '修改密码失败，请稍后重试',
    }
  }
}

/**
 * 修改用户邮箱
 * @param email 新邮箱
 * @returns { success: boolean, error?: string }
 */
export async function updateUserEmail(email: string) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth || auth.role !== 'user') {
      return {
        success: false,
        error: '未登录或登录已过期',
      }
    }

    // 2. 验证邮箱格式
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
    if (!emailRegex.test(email)) {
      return {
        success: false,
        error: '邮箱格式不正确',
      }
    }

    // 3. 更新数据库（仅本地）
    await prisma.user.update({
      where: { id: auth.id },
      data: { email },
    })

    // 4. 记录日志
    await prisma.log.create({
      data: {
        action: 'user_update_email',
        targetId: auth.id,
        details: {
          newEmail: email,
          timestamp: new Date().toISOString(),
        },
      },
    })

    return {
      success: true,
    }
  } catch (error) {
    console.error('修改邮箱失败：', error)
    return {
      success: false,
      error: '修改邮箱失败，请稍后重试',
    }
  }
}
