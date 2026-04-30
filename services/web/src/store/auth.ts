import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '@/api/auth'
import { useConsoleStore } from '@/store/console'
import { useUserStore } from '@/store/user'
import type { LoginCredentials, RegisterRequest, LoginResponse, RegisterResponse, LoginProtectionConfig } from '@/types/api'

const AUTH_TOKEN_KEY = 'token'
const AUTH_ROLE_KEY = 'role'
const AUTH_PASSWORD_RESET_REQUIRED_KEY = 'passwordResetRequired'

type AuthRole = 'admin' | 'user'
type CrossTabSyncReason = 'signed-out' | 'updated'

function readStorageItem(key: string): string | null {
  if (typeof window === 'undefined') {
    return null
  }

  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeStorageItem(key: string, value: string) {
  if (typeof window === 'undefined') {
    return
  }

  try {
    window.localStorage.setItem(key, value)
  } catch {
    // localStorage 不可用时退化为当前 tab 内存态。
  }
}

function removeStorageItem(key: string) {
  if (typeof window === 'undefined') {
    return
  }

  try {
    window.localStorage.removeItem(key)
  } catch {
    // localStorage 不可用时退化为当前 tab 内存态。
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(readStorageItem(AUTH_TOKEN_KEY))
  const role = ref<string | null>(readStorageItem(AUTH_ROLE_KEY))
  const passwordResetRequired = ref<boolean>(readStorageItem(AUTH_PASSWORD_RESET_REQUIRED_KEY) === 'true')
  const crossTabSyncEnabled = ref(false)
  let stopStorageSync: (() => void) | null = null

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => role.value === 'admin')
  const isUser = computed(() => role.value === 'user')

  const protectionConfig = ref<LoginProtectionConfig | null>(null)

  const loadProtectionConfig = async () => {
    if (protectionConfig.value) {
      return protectionConfig.value
    }
    const config = await authApi.getLoginProtectionConfig()
    protectionConfig.value = config
    return config
  }

  const login = async (credentials: LoginCredentials) => {
    useConsoleStore().clearConsoleData()
    const userStore = useUserStore()
    userStore.clearUserData()
    const res: LoginResponse = await authApi.login(credentials)
    setAuth(res.token, res.user.role as 'admin' | 'user', res.user.passwordResetRequired === true)
    userStore.setProfile(res.user)
    return res
  }

  const register = async (data: RegisterRequest) => {
    useConsoleStore().clearConsoleData()
    const userStore = useUserStore()
    userStore.clearUserData()
    const res: RegisterResponse = await authApi.register(data)
    setAuth(res.token, 'user', false)
    userStore.setProfile(res.user)
    return res
  }

  const logout = async () => {
    try {
      await authApi.logout()
    } finally {
      useConsoleStore().clearConsoleData()
      useUserStore().clearUserData()
      clearAuth()
    }
  }

  const setAuth = (newToken: string, newRole: AuthRole, requirePasswordReset = false) => {
    token.value = newToken
    role.value = newRole
    passwordResetRequired.value = requirePasswordReset
    writeStorageItem(AUTH_TOKEN_KEY, newToken)
    writeStorageItem(AUTH_ROLE_KEY, newRole)
    writeStorageItem(AUTH_PASSWORD_RESET_REQUIRED_KEY, requirePasswordReset ? 'true' : 'false')
  }

  const clearAuth = () => {
    token.value = null
    role.value = null
    passwordResetRequired.value = false
    removeStorageItem(AUTH_TOKEN_KEY)
    removeStorageItem(AUTH_ROLE_KEY)
    removeStorageItem(AUTH_PASSWORD_RESET_REQUIRED_KEY)
  }

  const setPasswordResetRequired = (required: boolean) => {
    passwordResetRequired.value = required
    writeStorageItem(AUTH_PASSWORD_RESET_REQUIRED_KEY, required ? 'true' : 'false')
  }

  const restoreAuth = () => {
    const savedToken = readStorageItem(AUTH_TOKEN_KEY)
    const savedRole = readStorageItem(AUTH_ROLE_KEY)
    const savedPasswordResetRequired = readStorageItem(AUTH_PASSWORD_RESET_REQUIRED_KEY)
    if (savedToken && savedRole) {
      token.value = savedToken
      role.value = savedRole
      passwordResetRequired.value = savedPasswordResetRequired === 'true'
      return
    }

    if (token.value || role.value) {
      return
    }

    token.value = null
    role.value = null
    passwordResetRequired.value = false
  }

  const syncAuthFromStorage = (): CrossTabSyncReason | null => {
    const savedToken = readStorageItem(AUTH_TOKEN_KEY)
    const savedRole = readStorageItem(AUTH_ROLE_KEY)
    const savedPasswordResetRequired = readStorageItem(AUTH_PASSWORD_RESET_REQUIRED_KEY)
    const previousToken = token.value
    const previousRole = role.value
    const previousPasswordResetRequired = passwordResetRequired.value
    const nextToken = savedToken && savedRole ? savedToken : null
    const nextRole = savedToken && savedRole ? savedRole : null
    const nextPasswordResetRequired = savedToken && savedRole ? savedPasswordResetRequired === 'true' : false

    if (previousToken === nextToken && previousRole === nextRole && previousPasswordResetRequired === nextPasswordResetRequired) {
      return null
    }

    token.value = nextToken
    role.value = nextRole
    passwordResetRequired.value = nextPasswordResetRequired
    useConsoleStore().clearConsoleData()
    useUserStore().clearUserData()

    if (!nextToken || !nextRole) {
      return previousToken || previousRole ? 'signed-out' : null
    }

    return 'updated'
  }

  const initCrossTabSync = (onChanged?: (reason: CrossTabSyncReason) => void) => {
    if (crossTabSyncEnabled.value || typeof window === 'undefined') {
      return
    }

    const handleStorage = (event: StorageEvent) => {
      if (event.storageArea !== window.localStorage) {
        return
      }

      if (
        event.key !== null &&
        event.key !== AUTH_TOKEN_KEY &&
        event.key !== AUTH_ROLE_KEY &&
        event.key !== AUTH_PASSWORD_RESET_REQUIRED_KEY
      ) {
        return
      }

      const reason = syncAuthFromStorage()
      if (reason && onChanged) {
        onChanged(reason)
      }
    }

    window.addEventListener('storage', handleStorage)
    crossTabSyncEnabled.value = true
    stopStorageSync = () => {
      window.removeEventListener('storage', handleStorage)
      crossTabSyncEnabled.value = false
      stopStorageSync = null
    }
  }

  const destroyCrossTabSync = () => {
    if (stopStorageSync) {
      stopStorageSync()
    }
  }

  return {
    token,
    role,
    crossTabSyncEnabled,
    isAuthenticated,
    isAdmin,
    isUser,
    passwordResetRequired,
    login,
    register,
    logout,
    setAuth,
    clearAuth,
    setPasswordResetRequired,
    restoreAuth,
    initCrossTabSync,
    destroyCrossTabSync,
    loadProtectionConfig,
    protectionConfig
  }
})
