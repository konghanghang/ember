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

  // 验证 token 是否有效
  let isAuthenticated = false
  if (token) {
    try {
      const secret = new TextEncoder().encode(JWT_SECRET)
      await jwtVerify(token, secret)
      isAuthenticated = true
    } catch {
      // Token 无效或过期
      isAuthenticated = false
    }
  }

  // 保护 /admin 路由：未登录重定向到登录页
  if (pathname.startsWith('/admin')) {
    if (!isAuthenticated) {
      const loginUrl = new URL('/login', request.url)
      // 记录用户想访问的页面，登录后可以跳转回来
      loginUrl.searchParams.set('redirect', pathname)
      return NextResponse.redirect(loginUrl)
    }
  }

  // 已登录用户访问登录页，重定向到管理后台
  if (pathname === '/login' && isAuthenticated) {
    // 检查是否有 redirect 参数
    const redirectTo = request.nextUrl.searchParams.get('redirect')
    const targetUrl = redirectTo || '/admin/invites'
    return NextResponse.redirect(new URL(targetUrl, request.url))
  }

  return NextResponse.next()
}

/**
 * 配置哪些路径需要经过中间件处理
 */
export const config = {
  matcher: [
    '/admin/:path*',  // 所有管理后台路由
    '/login',         // 登录页面
  ],
}
