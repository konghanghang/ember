import { useAuthStore } from './auth'
import { useConsoleStore } from './console'
import { useUserStore } from './user'

/**
 * 清空全部用户态 store 的运行时数据，等同于「未登录」内存态。
 *
 * 集中收口原本散落在 5 处的「clearConsoleData + clearUserData + clearAuth」重复逻辑：
 * auth.login、auth.register、auth.logout、auth.syncAuthFromStorage、main.ts 的 401 处理。
 *
 * 注意：本函数只清运行时态；localStorage 中已写入的 token 由 auth.clearAuth 内部移除。
 */
export function resetAllStores() {
  useAuthStore().clearAuth()
  useConsoleStore().clearConsoleData()
  useUserStore().clearUserData()
}
