import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as consoleApi from '@/api/console'
import { useAuthStore } from '@/store/auth'
import type {
  MediaStats,
  UserInfo
} from '@/types/api'

export const useUserStore = defineStore('user', () => {
  const profile = ref<UserInfo | null>(null)
  const mediaStats = ref<MediaStats | null>(null)
  const embyUrl = ref<string>('')
  // embyConfigured 表示系统级 Emby 集成是否齐全（URL + API Key）。
  // 默认 true，避免首次进入 dashboard 在探测请求返回前误显示"未配置"空态。
  const embyConfigured = ref<boolean>(true)

  const setProfile = (user: UserInfo | null) => {
    profile.value = user
  }

  const setEmbyUrl = (url: string) => {
    embyUrl.value = url
  }

  const setEmbyConfigured = (configured: boolean) => {
    embyConfigured.value = configured
  }

  const clearEmbyUrl = () => {
    embyUrl.value = ''
  }

  const fetchProfile = async () => {
    const res = await consoleApi.getProfile()
    setProfile(res)
    useAuthStore().setSessionFromProfile(res)
    return res
  }

  const updateEmail = async (newEmail: string, code: string) => {
    await consoleApi.updateEmail(newEmail, code)
    if (profile.value) {
      profile.value.email = newEmail
    }
  }

  const updatePassword = async (oldPassword: string, newPassword: string) => {
    await consoleApi.updatePassword({ oldPassword, newPassword })
    useAuthStore().setPasswordResetRequired(false)
    if (profile.value) {
      profile.value.passwordResetRequired = false
    }
  }

  const fetchMediaStats = async () => {
    const res = await consoleApi.getMediaStats()
    mediaStats.value = res.data
    return res.data
  }

  const fetchEmbyConfig = async () => {
    const res = await consoleApi.getEmbyConfig()
    // 兼容：老协议无 configured 字段时按 url 是否为空兜底，确保渐进升级期间行为一致。
    const configured = res.configured ?? Boolean(res.url)
    setEmbyConfigured(configured)
    setEmbyUrl(configured ? res.url : '')
    return res
  }

  const clearUserData = () => {
    profile.value = null
    mediaStats.value = null
    clearEmbyUrl()
    embyConfigured.value = true
  }

  return {
    profile,
    mediaStats,
    embyUrl,
    embyConfigured,
    setProfile,
    setEmbyUrl,
    setEmbyConfigured,
    clearEmbyUrl,

    fetchProfile,
    updateEmail,
    updatePassword,
    fetchMediaStats,
    fetchEmbyConfig,
    clearUserData
  }
})
