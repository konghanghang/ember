import request from './request'

// 管理员登录
export function login(data: any) {
  return request({
    url: '/admin/login',
    method: 'post',
    data
  })
}

// 获取当前管理员信息
export function getCurrentAdmin() {
  return request({
    url: '/admin/current',
    method: 'get'
  })
}

// 管理员登出
export function adminLogout() {
  return request({
    url: '/admin/logout',
    method: 'post'
  })
}

// 用户登录
export function userLogin(data: any) {
  return request({
    url: '/user/login',
    method: 'post',
    data
  })
}

// 用户登出
export function userLogout() {
  return request({
    url: '/user/logout',
    method: 'post'
  })
}

// 用户注册
export function register(data: any) {
  return request({
    url: '/user/register',
    method: 'post',
    data
  })
}

// 验证邀请码
export function validateInviteCode(code: string) {
  return request({
    url: `/invites/${code}/validate`,
    method: 'get'
  })
}

// 兼容旧代码的 logout 函数
export function logout() {
  // 根据当前路径判断是管理员还是用户
  const isAdmin = window.location.pathname.startsWith('/admin')
  return isAdmin ? adminLogout() : userLogout()
}
