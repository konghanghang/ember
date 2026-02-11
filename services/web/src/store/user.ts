import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as userApi from '@/api/user'
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

  const fetchProfile = async () => {
    const res = await userApi.getUserProfile()
    profile.value = res
    return res
  }

  const updateEmail = async (newEmail: string) => {
    await userApi.updateUserEmail(newEmail)
    if (profile.value) {
      profile.value.email = newEmail
    }
  }

  const updatePassword = async (oldPassword: string, newPassword: string) => {
    await userApi.updateUserPassword({ oldPassword, newPassword })
  }

  const fetchSubscriptions = async () => {
    const res = await userApi.getUserSubscriptions()
    subscriptions.value = res
    return res
  }

  const createSubscription = async (data: CreateSubscriptionRequest) => {
    await userApi.createSubscription(data)
    await fetchSubscriptions()
  }

  const deleteSubscription = async (id: string) => {
    await userApi.deleteSubscription(id)
    subscriptions.value = subscriptions.value.filter(s => s.id !== id)
  }

  const fetchMediaStats = async () => {
    const res = await userApi.getMediaStats()
    mediaStats.value = res.data
    return res.data
  }

  const fetchEmbyConfig = async () => {
    const res = await userApi.getEmbyConfig()
    embyUrl.value = res.url
    return res.url
  }

  const clearUserData = () => {
    profile.value = null
    subscriptions.value = []
    mediaStats.value = null
    embyUrl.value = ''
  }

  return {
    profile,
    subscriptions,
    mediaStats,
    embyUrl,
    
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
