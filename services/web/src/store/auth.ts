import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '@/api/auth'
import { useConsoleStore } from '@/store/console'
import { useUserStore } from '@/store/user'
import { resetAllStores } from '@/store/reset'
import type { LoginCredentials, RegisterRequest, LoginResponse, RegisterResponse, LoginProtectionConfig, UserInfo } from '@/types/api'

const AUTH_TOKEN_KEY = 'token'
const LEGACY_AUTH_ROLE_KEY = 'role'
const LEGACY_AUTH_PASSWORD_RESET_REQUIRED_KEY = 'passwordResetRequired'

type AuthRole = 'admin' | 'user'
type CrossTabSyncReason = 'signed-out' | 'updated'

// 登录保护配置缓存 TTL：admin 后台调整 Turnstile 配置后，最多 5 分钟内自动生效。
const PROTECTION_CONFIG_TTL_MS = 5 * 60 * 1000

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

/**
 * 探测 localStorage 是否可读写。
 *
 * sandboxed iframe、隐私模式或浏览器策略可能让 localStorage 抛 SecurityError。此时
 * login/register 的 writeStorageItem 已退化为内存会话；restoreAuth 必须能区分「存储不可用」
 * 与「token 键确实不存在」，否则会把刚登录的内存会话当作未登录清掉。
 */
function isLocalStorageAvailable(): boolean {
  if (typeof window === 'undefined') {
    return false
  }

  try {
    const probe = '__ember_auth_probe__'
    window.localStorage.setItem(probe, probe)
    window.localStorage.removeItem(probe)
    return true
  } catch {
    return false
  }
}

export const useAuthStore = defineStore('auth', () => {
  // 单一事实源：token 由 auth store 持有（写 localStorage），role 与 passwordResetRequired
  // 从 user store.profile 派生（消除 P2-5 双份存储）。login/register/logout 通过 resetAllStores
  // 与 setProfile 统一更新两个 store，避免字段被多处手动同步。
  const token = ref<string | null>(readStorageItem(AUTH_TOKEN_KEY))
  const userStore = useUserStore()

  const role = computed<AuthRole | null>(() => (userStore.profile?.role as AuthRole | undefined) ?? null)
  const passwordResetRequired = computed<boolean>(() => userStore.profile?.passwordResetRequired === true)

  const crossTabSyncEnabled = ref(false)
  let stopStorageSync: (() => void) | null = null

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => role.value === 'admin')
  const isUser = computed(() => role.value === 'user')

  const protectionConfig = ref<LoginProtectionConfig | null>(null)
  // protectionConfig 的写入时间戳；0 表示当前为「未加载或已失效」状态。
  let protectionConfigLoadedAt = 0

  /**
   * 拉取登录保护配置（Turnstile 等）。
   *
   * 缓存语义：成功加载后保留 PROTECTION_CONFIG_TTL_MS（5min）；TTL 内重复调用命中缓存，
   * 过期或 force=true 时重新拉取。invalidateProtectionConfig 提供显式失效入口，
   * 供 admin 后台改配置后强制刷新使用——修复 P3 永久缓存无失效路径的问题。
   */
  const loadProtectionConfig = async ({ force = false }: { force?: boolean } = {}) => {
    const now = Date.now()
    const fresh = protectionConfigLoadedAt > 0 && (now - protectionConfigLoadedAt) < PROTECTION_CONFIG_TTL_MS
    if (protectionConfig.value && fresh && !force) {
      return protectionConfig.value
    }

    const config = await authApi.getLoginProtectionConfig()
    protectionConfig.value = config
    protectionConfigLoadedAt = now
    return config
  }

  /**
   * 显式失效登录保护配置缓存。下次 loadProtectionConfig 会重新拉取。
   */
  const invalidateProtectionConfig = () => {
    protectionConfig.value = null
    protectionConfigLoadedAt = 0
  }

  const login = async (credentials: LoginCredentials) => {
    // 重置全部用户态 store，确保新会话不被旧数据污染。
    resetAllStores()
    const res: LoginResponse = await authApi.login(credentials)
    token.value = res.token
    writeStorageItem(AUTH_TOKEN_KEY, res.token)
    userStore.setProfile(res.user)
    return res
  }

  const register = async (data: RegisterRequest) => {
    // 注册成功后再切换会话：已登录用户进入未受保护的 /register 并提交一个会失败的注册请求时，
    // 不能在请求发出前就清掉当前 token，否则失败后用户被意外登出。
    const res: RegisterResponse = await authApi.register(data)
    resetAllStores()
    token.value = res.token
    writeStorageItem(AUTH_TOKEN_KEY, res.token)
    userStore.setProfile(res.user)
    return res
  }

  const logout = async () => {
    try {
      await authApi.logout()
    } finally {
      resetAllStores()
    }
  }

  const clearLegacyAuthStorage = () => {
    removeStorageItem(LEGACY_AUTH_ROLE_KEY)
    removeStorageItem(LEGACY_AUTH_PASSWORD_RESET_REQUIRED_KEY)
  }

  /**
   * 写入 token 与配套 profile。profile 为可选——仅设置 token 时（如外部已自行写 profile）
   * role/passwordResetRequired 由当前 user store.profile 派生得出。
   */
  const setAuth = (newToken: string, profile?: Pick<UserInfo, 'role' | 'passwordResetRequired'> | null) => {
    token.value = newToken
    writeStorageItem(AUTH_TOKEN_KEY, newToken)
    if (profile) {
      // 合并到现有 profile：保持 UserInfo 其余字段（id、email 等）不被覆盖。
      userStore.setProfile({ ...(userStore.profile ?? null), ...profile } as UserInfo | null)
    }
    clearLegacyAuthStorage()
  }

  /**
   * 清理 auth 自身持有的最小状态：token 与 localStorage。profile/console 由 resetAllStores 协同清理。
   */
  const clearAuth = () => {
    token.value = null
    removeStorageItem(AUTH_TOKEN_KEY)
    clearLegacyAuthStorage()
  }

  /**
   * 显式更新 passwordResetRequired 标记（写单一事实源：user store.profile）。
   * 维持对外 setter 语义不变。profile 缺失时为 no-op，与「未登录」状态一致。
   */
  const setPasswordResetRequired = (required: boolean) => {
    if (userStore.profile) {
      userStore.profile.passwordResetRequired = required
    }
  }

  /**
   * 应用启动期从 localStorage 恢复 token。
   *
   * 修复 P3「幽灵登录」：旧实现在「内存有 token 但 storage 没有」时早退，残留的内存态
   * 会让用户在未真正登录的情况下被识别为已登录。新实现统一以 storage 为准：storage 可读
   * 且无 token 即视为未登录，强制清空 token + profile。
   *
   * 但 localStorage 不可用时（sandboxed iframe / 隐私模式，读写抛 SecurityError），
   * login/register 已退化为内存会话——此时不能清场，否则会把刚登录的用户送回登录页。
   */
  const restoreAuth = () => {
    if (!isLocalStorageAvailable()) {
      // 存储不可用：保留内存会话，仅尽力清理 legacy key（不可读时自然 no-op）。
      clearLegacyAuthStorage()
      return
    }

    const savedToken = readStorageItem(AUTH_TOKEN_KEY)
    clearLegacyAuthStorage()
    token.value = savedToken
    if (!savedToken) {
      // storage 可读且无 token：彻底清场，杜绝幽灵登录残留。
      userStore.clearUserData()
    }
  }

  /**
   * 跨 tab 同步：依据 localStorage 当前 token 推导本 tab 应有状态。
   * 调用方依据返回的 reason 决定是否提示用户/跳转。
   */
  const syncAuthFromStorage = (): CrossTabSyncReason | null => {
    const savedToken = readStorageItem(AUTH_TOKEN_KEY)
    const previousToken = token.value
    const nextToken = savedToken || null

    // token 未变且派生 role 也无残留（profile 已清）时跳过。
    const previousProfileExists = !!userStore.profile
    if (previousToken === nextToken && (nextToken || !previousProfileExists)) {
      return null
    }

    token.value = nextToken
    // token 变化时清掉 user/console 派生状态，role/passwordResetRequired 随之置空。
    userStore.clearUserData()
    useConsoleStore().clearConsoleData()

    if (!nextToken) {
      return previousToken ? 'signed-out' : null
    }

    return 'updated'
  }

  /**
   * 启动跨 tab 同步监听。
   *
   * 响应两类 storage 事件：
   * - `event.key === AUTH_TOKEN_KEY`：token 被其他 tab 写入/移除；
   * - `event.key === null`：其他 tab 调用了 `localStorage.clear()`，token 随之被删，
   *   本 tab 必须重新读取并同步登出，否则会保留失效会话直到下次导航或 401。
   *
   * 注意：`key === null` 只有 `clear()` 会触发；`removeItem(tokenKey)` 触发的是
   * `key === AUTH_TOKEN_KEY` 事件，不会落到 null 分支。
   */
  const initCrossTabSync = (onChanged?: (reason: CrossTabSyncReason) => void) => {
    if (crossTabSyncEnabled.value || typeof window === 'undefined') {
      return
    }

    const handleStorage = (event: StorageEvent) => {
      if (event.storageArea !== window.localStorage) {
        return
      }

      // AUTH_TOKEN_KEY 变化或 clear()（key===null）都需要重新读取并同步。
      if (event.key !== AUTH_TOKEN_KEY && event.key !== null) {
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
    invalidateProtectionConfig,
    protectionConfig
  }
})
