import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as adminApi from '@/api/admin'
import * as authApi from '@/api/auth'

interface AdminInfo {
  id: string
  username: string
  createdAt: string
}

export const useAdminStore = defineStore('admin', () => {
  const info = ref<AdminInfo | null>(null)

  const fetchCurrentAdmin = async () => {
    const res: any = await authApi.getCurrentAdmin()
    info.value = res
    return res
  }

  const fetchSystemInfo = async () => {
    return await adminApi.getSystemInfo()
  }

  const testEmbyConnection = async () => {
    return await adminApi.testEmbyConnection()
  }

  const runCronJob = async () => {
    return await adminApi.runCronJob()
  }

  const clearAdminData = () => {
    info.value = null
  }

  return {
    info,
    
    fetchCurrentAdmin,
    fetchSystemInfo,
    testEmbyConnection,
    runCronJob,
    clearAdminData
  }
})
