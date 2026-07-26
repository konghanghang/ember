import { request } from './request'
import type {
  AdminInfo,
  LoginCredentials,
  LoginResponse,
  LoginProtectionConfig,
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

export function getLoginProtectionConfig(): Promise<LoginProtectionConfig> {
  return request({
    url: '/login/protection-config',
    method: 'get'
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

// 发送邮箱验证码
export function sendEmailCode(email: string): Promise<{ message: string }> {
  return request({
    url: '/register/send-code',
    method: 'post',
    data: { email }
  })
}

// 发送密码重置验证码
export function sendResetCode(email: string): Promise<{ message: string }> {
  return request({
    url: '/forgot-password/send-code',
    method: 'post',
    data: { email }
  })
}

// 通过验证码重置密码
export function resetPasswordByCode(data: {
  email: string
  code: string
  newPassword: string
}): Promise<{ message: string }> {
  return request({
    url: '/forgot-password/reset',
    method: 'post',
    data
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
