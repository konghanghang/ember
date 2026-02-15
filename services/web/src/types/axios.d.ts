import 'axios'

declare module 'axios' {
  export interface AxiosRequestConfig {
    // If true, suppress global ElMessage.error for non-401 errors.
    silent?: boolean
  }
}

