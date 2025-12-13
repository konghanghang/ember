import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { jwtVerify } from 'jose'

const JWT_SECRET = process.env.JWT_SECRET || 'default-secret-key-change-this-in-production'

/**
 * 中间件：统一处理路由保护和认证逻辑
 */
export async function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl
  const token = request.cookies.get('auth-token')?.value

  // 验证 token 并提取角色信息
  let auth: { role: 'admin' | 'user' } | null = null
  if (token) {
    try {
      const secret = new TextEncoder().encode(JWT_SECRET)
      const { payload } = await jwtVerify(token, secret)
      // 向后兼容：旧 token 没有 role 字段，默认为 admin
      auth = { role: (payload.role as 'admin' | 'user') || 'admin' }
    } catch {
      // Token 无效或过期
      auth = null
    }
  }

  // 保护 /admin/* 路由：需要管理员权限
  if (pathname.startsWith('/admin')) {
    if (!auth || auth.role !== 'admin') {
      const loginUrl = new URL('/login', request.url)
      loginUrl.searchParams.set('redirect', pathname)
      return NextResponse.redirect(loginUrl)
    }
  }

  // 保护 /user/* 路由（除了登录页）：需要用户权限
  if (pathname.startsWith('/user') && pathname !== '/user/login') {
    if (!auth || auth.role !== 'user') {
      const loginUrl = new URL('/user/login', request.url)
      loginUrl.searchParams.set('redirect', pathname)
      return NextResponse.redirect(loginUrl)
    }
  }

  // 已登录管理员访问管理员登录页，重定向到管理后台
  if (pathname === '/login' && auth && auth.role === 'admin') {
    const redirectTo = request.nextUrl.searchParams.get('redirect')
    const targetUrl = redirectTo || '/admin/invites'
    return NextResponse.redirect(new URL(targetUrl, request.url))
  }

  // 已登录用户访问用户登录页，重定向到用户仪表盘
  if (pathname === '/user/login' && auth && auth.role === 'user') {
    const redirectTo = request.nextUrl.searchParams.get('redirect')
    const targetUrl = redirectTo || '/user/dashboard'
    return NextResponse.redirect(new URL(targetUrl, request.url))
  }

  return NextResponse.next()
}

/**
 * 配置哪些路径需要经过中间件处理
 */
export const config = {
  matcher: [
    '/admin/:path*',     // 所有管理后台路由
    '/user/:path*',      // 所有用户路由
    '/login',            // 管理员登录页
  ],
}
