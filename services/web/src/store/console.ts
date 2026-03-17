import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getConsoleAccountLinks } from '@/api/console'
import type { ConsoleAccountLink } from '@/types/api'

export const useConsoleStore = defineStore('console', () => {
  const accountLinks = ref<ConsoleAccountLink[]>([])

  const fetchAccountLinks = async () => {
    const res = await getConsoleAccountLinks()
    accountLinks.value = res.data || []
    return accountLinks.value
  }

  const clearConsoleData = () => {
    accountLinks.value = []
  }

  return {
    accountLinks,
    fetchAccountLinks,
    clearConsoleData
  }
})
