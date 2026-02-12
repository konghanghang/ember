import request from './request'
import type {
  AdminInfo,
  LoginCredentials,
  LoginResponse,
  RedemptionCode,
  RegistrationModeResponse,
  RegisterRequest,
  RegisterResponse
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

export function getRegistrationMode(): Promise<RegistrationModeResponse> {
  return request({
    url: '/register/mode',
    method: 'get'
  })
}

export function validateRegistrationCode(code: string): Promise<RedemptionCode> {
  return request({
    url: `/register/code/${code}/validate`,
    method: 'get'
  })
}

// 获取当前管理员信息
export async function getCurrentAdmin(): Promise<AdminInfo> {
  const response = await request<{ user: AdminInfo }>({
    url: '/admin/current',
    method: 'get'
  })
  return response.user
}
