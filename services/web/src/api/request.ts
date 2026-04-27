import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'
import { useConsoleStore } from '@/store/console'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'

let handlingUnauthorized = false

const service = axios.create({
  baseURL: '/api/v1',
  timeout: 60000,
})

function isAuthEndpoint(url: string | undefined, endpoint: 'login' | 'logout') {
  return typeof url === 'string' && new RegExp(`(^|/)${endpoint}(?:/|\\?|$)`).test(url)
}

function getUnauthorizedRedirectTarget() {
  const currentRoute = router.currentRoute.value
  if (!currentRoute || currentRoute.path === '/login') {
    return '/console/dashboard'
  }

  return currentRoute.fullPath
}

async function handleUnauthorizedRedirect() {
  if (handlingUnauthorized) {
    return
  }

  handlingUnauthorized = true

  try {
    const authStore = useAuthStore()
    const consoleStore = useConsoleStore()
    const userStore = useUserStore()
    const redirect = getUnauthorizedRedirectTarget()

    consoleStore.clearConsoleData()
    userStore.clearUserData()
    authStore.clearAuth()
    ElMessage.error('登录已过期，请重新登录')

    await router.push({
      path: '/login',
      query: { redirect }
    })
  } finally {
    handlingUnauthorized = false
  }
}

service.interceptors.request.use(
  (config) => {
    const authStore = useAuthStore()
    if (authStore.token) {
      config.headers['Authorization'] = `Bearer ${authStore.token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

service.interceptors.response.use(
  (response) => {
    return response.data
  },
  (error) => {
    const status = error.response?.status
    const message = error.response?.data?.error || '请求失败'
    const silent = error.config?.silent === true

    if (status === 401) {
      if (isAuthEndpoint(error.config?.url, 'logout')) {
        return Promise.reject(error)
      }

      if (isAuthEndpoint(error.config?.url, 'login')) {
        if (!silent) {
          ElMessage.error(message)
        }
      } else {
        void handleUnauthorizedRedirect()
      }
    } else {
      if (!silent) {
        ElMessage.error(message)
      }
    }
    
    return Promise.reject(error)
  }
)

export default service
