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

  const setProfile = (user: UserInfo | null) => {
    profile.value = user
  }

  const setEmbyUrl = (url: string) => {
    embyUrl.value = url
  }

  const clearEmbyUrl = () => {
    embyUrl.value = ''
  }

  const fetchProfile = async () => {
    const res = await consoleApi.getProfile()
    setProfile(res)
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
    setEmbyUrl(res.url)
    return res.url
  }

  const clearUserData = () => {
    profile.value = null
    mediaStats.value = null
    clearEmbyUrl()
  }

  return {
    profile,
    mediaStats,
    embyUrl,
    setProfile,
    setEmbyUrl,
    clearEmbyUrl,
    
    fetchProfile,
    updateEmail,
    updatePassword,
    fetchMediaStats,
    fetchEmbyConfig,
    clearUserData
  }
})
