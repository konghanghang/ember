import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '@/api/auth'
import type { LoginCredentials, RegisterRequest, AdminLoginResponse, UserLoginResponse, RegisterResponse } from '@/types/api'

export const useAuthStore = defineStore('auth', () => {
  // 状态
  const token = ref<string | null>(localStorage.getItem('token'))
  const role = ref<string | null>(localStorage.getItem('role'))

  // 计算属性
  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => role.value === 'admin')
  const isUser = computed(() => role.value === 'user')

  // Actions
  
  /**
   * 管理员登录
   */
  const adminLogin = async (credentials: LoginCredentials) => {
    const res: AdminLoginResponse = await authApi.login(credentials)
    setAuth(res.token, 'admin')
    return res
  }

  /**
   * 用户登录
   */
  const userLogin = async (credentials: LoginCredentials) => {
    const res: UserLoginResponse = await authApi.userLogin(credentials)
    setAuth(res.token, 'user')
    return res
  }

  /**
   * 用户注册
   */
  const register = async (data: RegisterRequest) => {
    const res: RegisterResponse = await authApi.register(data)
    setAuth(res.token, 'user')
    return res
  }

  /**
   * 登出
   */
  const logout = async () => {
    try {
      await authApi.logout()
    } finally {
      clearAuth()
    }
  }

  /**
   * 设置认证信息
   */
  const setAuth = (newToken: string, newRole: 'admin' | 'user') => {
    token.value = newToken
    role.value = newRole
    localStorage.setItem('token', newToken)
    localStorage.setItem('role', newRole)
  }

  /**
   * 清除认证信息
   */
  const clearAuth = () => {
    token.value = null
    role.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('role')
  }

  /**
   * 从 localStorage 恢复认证状态
   */
  const restoreAuth = () => {
    const savedToken = localStorage.getItem('token')
    const savedRole = localStorage.getItem('role')
    if (savedToken && savedRole) {
      token.value = savedToken
      role.value = savedRole
    }
  }

  /**
   * 验证邀请码
   */
  const validateInvite = async (code: string) => {
    return await authApi.validateInviteCode(code)
  }

  return {
    // State
    token,
    role,
    
    // Getters
    isAuthenticated,
    isAdmin,
    isUser,
    
    // Actions
    adminLogin,
    userLogin,
    register,
    logout,
    setAuth,
    clearAuth,
    restoreAuth,
    validateInvite
  }
})
