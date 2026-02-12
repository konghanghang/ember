import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '@/api/auth'
import type { LoginCredentials, RegisterRequest, LoginResponse, RegisterResponse } from '@/types/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('token'))
  const role = ref<string | null>(localStorage.getItem('role'))

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => role.value === 'admin')
  const isUser = computed(() => role.value === 'user')

  const login = async (credentials: LoginCredentials) => {
    const res: LoginResponse = await authApi.login(credentials)
    setAuth(res.token, res.user.role as 'admin' | 'user')
    return res
  }

  const register = async (data: RegisterRequest) => {
    const res: RegisterResponse = await authApi.register(data)
    setAuth(res.token, 'user')
    return res
  }

  const logout = async () => {
    try {
      await authApi.logout()
    } finally {
      clearAuth()
    }
  }

  const setAuth = (newToken: string, newRole: 'admin' | 'user') => {
    token.value = newToken
    role.value = newRole
    localStorage.setItem('token', newToken)
    localStorage.setItem('role', newRole)
  }

  const clearAuth = () => {
    token.value = null
    role.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('role')
  }

  const restoreAuth = () => {
    const savedToken = localStorage.getItem('token')
    const savedRole = localStorage.getItem('role')
    if (savedToken && savedRole) {
      token.value = savedToken
      role.value = savedRole
    }
  }

  return {
    token,
    role,
    isAuthenticated,
    isAdmin,
    isUser,
    login,
    register,
    logout,
    setAuth,
    clearAuth,
    restoreAuth
  }
})
