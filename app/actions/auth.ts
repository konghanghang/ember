'use server'

import { cookies } from 'next/headers'
import { redirect } from 'next/navigation'
import { prisma } from '@/lib/db'
import { signToken, verifyPassword, getServerAuth } from '@/lib/auth'
import { validatePasswordStrength, hashPassword } from '@/lib/auth-helpers'

/**
 * 管理员登录
 * @param data 登录信息 { username, password }
 * @returns { success: boolean, error?: string } 或直接 redirect
 */
export async function adminLogin(data: { username: string; password: string }) {
  try {
    console.log('[登录] 开始登录验证:', { username: data.username })

    // 1. 查找管理员
    const admin = await prisma.admin.findUnique({
      where: { username: data.username },
    })

    console.log('[登录] 数据库查询结果:', {
      found: !!admin,
      username: admin?.username,
      hasPassword: !!admin?.password,
    })

    // 2. 验证管理员存在性
    if (!admin) {
      console.log('[登录] 失败 - 用户不存在:', data.username)
      return {
        success: false,
        error: '用户名或密码错误',
      }
    }

    // 3. 验证密码
    const isPasswordValid = await verifyPassword(data.password, admin.password)
    console.log('[登录] 密码验证结果:', { isPasswordValid })

    if (!isPasswordValid) {
      console.log('[登录] 失败 - 密码错误')
      return {
        success: false,
        error: '用户名或密码错误',
      }
    }

    console.log('[登录] 验证成功，生成 Token')

    // 4. 生成 JWT Token
    const token = await signToken({
      id: admin.id,
      username: admin.username,
      role: 'admin',
    })

    // 5. 设置 httpOnly cookie（安全存储）
    const cookieStore = await cookies()
    cookieStore.set('auth-token', token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 7, // 7天
      path: '/',
    })

    // 6. 记录登录日志
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

    console.log('[登录] 登录成功，重定向到管理后台')

    // 7. 重定向到管理后台（这会抛出 NEXT_REDIRECT，属于正常流程）
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

/**
 * 修改管理员密码
 * @param data { currentPassword, newPassword }
 * @returns { success: boolean, error?: string }
 */
export async function updateAdminPassword(data: {
  currentPassword: string
  newPassword: string
}) {
  try {
    // 1. 获取当前登录用户
    const auth = await getServerAuth()
    if (!auth) {
      return {
        success: false,
        error: '未登录或登录已过期',
      }
    }

    // 2. 验证新密码强度
    const passwordValidation = validatePasswordStrength(data.newPassword)
    if (!passwordValidation.valid) {
      return {
        success: false,
        error: passwordValidation.reason || '密码不符合要求',
      }
    }

    // 3. 查找管理员
    const admin = await prisma.admin.findUnique({
      where: { id: auth.id },
    })

    if (!admin) {
      return {
        success: false,
        error: '管理员账号不存在',
      }
    }

    // 4. 验证当前密码
    const isCurrentPasswordValid = await verifyPassword(
      data.currentPassword,
      admin.password
    )

    if (!isCurrentPasswordValid) {
      return {
        success: false,
        error: '当前密码错误',
      }
    }

    // 5. 加密新密码
    const hashedPassword = await hashPassword(data.newPassword, 10)

    // 6. 更新密码
    await prisma.admin.update({
      where: { id: auth.id },
      data: {
        password: hashedPassword,
      },
    })

    // 7. 记录日志
    await prisma.log.create({
      data: {
        action: 'update_password',
        targetId: auth.id,
        details: {
          username: admin.username,
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
