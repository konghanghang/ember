import request from './request'
import type {
  LoginCredentials,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
  ValidateInviteResponse
} from '@/types/api'

// 统一登录
export function login(data: LoginCredentials): Promise<LoginResponse> {
  return request({
    url: '/login',
    method: 'post',
    data
  })
}

// 统一登出（不区分角色）
export function logout() {
  return request({ url: '/logout', method: 'post' })
}

// 用户注册
export function register(data: RegisterRequest): Promise<RegisterResponse> {
  return request({
    url: '/user/register',
    method: 'post',
    data
  })
}

// 验证邀请码
export function validateInviteCode(code: string): Promise<ValidateInviteResponse> {
  return request({
    url: `/invites/${code}/validate`,
    method: 'get'
  })
}
