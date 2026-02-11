import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as userApi from '@/api/user'

interface UserProfile {
  id: string
  username: string
  email: string
  embyId: string
  expiresAt: string
  isActive: boolean
  createdAt: string
}

interface Subscription {
  id: string
  type: string
  name: string
  tmdbId: string
  status: string
  posterPath?: string
  note?: string
  mpError?: string
  createdAt: string
}

interface MediaStats {
  MovieCount: number
  SeriesCount: number
  EpisodeCount: number
}

export const useUserStore = defineStore('user', () => {
  const profile = ref<UserProfile | null>(null)
  const subscriptions = ref<Subscription[]>([])
  const mediaStats = ref<MediaStats | null>(null)
  const embyUrl = ref<string>('')

  const fetchProfile = async () => {
    const res: any = await userApi.getUserProfile()
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
    const res: any = await userApi.getUserSubscriptions()
    subscriptions.value = res
    return res
  }

  const createSubscription = async (data: {
    type: string
    name: string
    tmdbId: string
    posterPath?: string
    note?: string
  }) => {
    await userApi.createSubscription(data)
    await fetchSubscriptions()
  }

  const deleteSubscription = async (id: string) => {
    await userApi.deleteSubscription(id)
    subscriptions.value = subscriptions.value.filter(s => s.id !== id)
  }

  const fetchMediaStats = async () => {
    const res: any = await userApi.getMediaStats()
    mediaStats.value = res.data
    return res.data
  }

  const fetchEmbyConfig = async () => {
    const res: any = await userApi.getEmbyConfig()
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
