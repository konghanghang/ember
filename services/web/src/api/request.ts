import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'
import { useAuthStore } from '@/store/auth'

const service = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

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
      ElMessage.error('登录已过期，请重新登录')
      const authStore = useAuthStore()
      authStore.clearAuth()
      router.push('/login')
    } else {
      if (!silent) {
        ElMessage.error(message)
      }
    }
    
    return Promise.reject(error)
  }
)

export default service
