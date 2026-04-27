import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as consoleApi from '@/api/console'
import type {
  CreateSubscriptionRequest,
  MediaStats,
  Subscription,
  UserInfo
} from '@/types/api'

export const useUserStore = defineStore('user', () => {
  const profile = ref<UserInfo | null>(null)
  const subscriptions = ref<Subscription[]>([])
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

  const updateEmail = async (newEmail: string) => {
    await consoleApi.updateEmail(newEmail)
    if (profile.value) {
      profile.value.email = newEmail
    }
  }

  const updatePassword = async (oldPassword: string, newPassword: string) => {
    await consoleApi.updatePassword({ oldPassword, newPassword })
  }

  const fetchSubscriptions = async () => {
    const res = await consoleApi.getSubscriptions({ page: 1, pageSize: 100 })
    subscriptions.value = res.data || []
    return subscriptions.value
  }

  const createSubscription = async (data: CreateSubscriptionRequest) => {
    await consoleApi.createSubscription(data)
    await fetchSubscriptions()
  }

  const deleteSubscription = async (id: string) => {
    await consoleApi.deleteSubscription(id)
    subscriptions.value = subscriptions.value.filter(s => s.id !== id)
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
    subscriptions.value = []
    mediaStats.value = null
    clearEmbyUrl()
  }

  return {
    profile,
    subscriptions,
    mediaStats,
    embyUrl,
    setProfile,
    setEmbyUrl,
    clearEmbyUrl,
    
    fetchProfile,
    updateEmail,
    updatePassword,
    fetchSubscriptions,
    createSubscription,
    deleteSubscription,
    fetchMediaStats,
    fetchEmbyConfig,
    clearUserData
  }
})
