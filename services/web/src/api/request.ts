import axios from 'axios'
import type { AxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'

/**
 * 请求层依赖注入入口（P2-6：斩断 request ↔ store/router 模块级循环依赖）。
 *
 * 旧实现直接 `import router from '../router'` 与 `import { useAuthStore } from '@/store/auth'`，
 * 形成 request → store/auth → api/auth → request 的环，仅靠拦截器回调延迟调用侥幸可用。
 *
 * 新实现只持有函数引用：main.ts 启动期调用 setupRequestInterceptors 装配 token getter
 * 与 unauthorized 回调；本模块不再 import store/router，环被斩断。
 */
interface RequestInterceptorsOptions {
  /** 请求拦截器读 token，写入 Authorization 头。 */
  getToken: () => string | null
  /** 非 login/logout 端点收到 401 时调用，由调用方负责清理 store + 跳转登录页。 */
  onUnauthorized: () => void | Promise<void>
}

let handlingUnauthorized = false
let tokenGetter: () => string | null = () => null
let unauthorizedHandler: (() => void | Promise<void>) | null = null

const service = axios.create({
  baseURL: '/api/v1',
  timeout: 60000,
})

function isAuthEndpoint(url: string | undefined, endpoint: 'login' | 'logout') {
  return typeof url === 'string' && new RegExp(`(^|/)${endpoint}(?:/|\\?|$)`).test(url)
}

/**
 * 处理 401：race lock 保证并发 401 只触发一次回调。
 * lock 在同步进入 handler 时即置位，跨 await 持有直到完成；任何后续 401 直接早退。
 */
async function handleUnauthorized() {
  if (handlingUnauthorized || !unauthorizedHandler) {
    return
  }

  handlingUnauthorized = true

  try {
    await unauthorizedHandler()
  } finally {
    handlingUnauthorized = false
  }
}

service.interceptors.request.use(
  (config) => {
    const token = tokenGetter()
    if (token) {
      config.headers['Authorization'] = `Bearer ${token}`
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
        void handleUnauthorized()
      }
    } else {
      if (!silent) {
        ElMessage.error(message)
      }
    }

    return Promise.reject(error)
  }
)

/**
 * 装配请求拦截器依赖。在 main.ts 初始化阶段调用一次。
 */
export function setupRequestInterceptors(options: RequestInterceptorsOptions) {
  tokenGetter = options.getToken
  unauthorizedHandler = options.onUnauthorized
}

/**
 * 发起 API 请求并 resolve 为已解包的响应体。
 *
 * service 的响应拦截器在运行时返回 `response.data`，本函数利用 axios 的
 * `R` 泛型把"返回已解包数据"这一事实编码进类型签名：`T` 即响应体类型
 * （blob 下载等 `responseType` 场景同样成立，此时 `response.data` 即 Blob），
 * 调用方不再需要 `as unknown as` 强转。
 *
 * 注意：这里描述的只是拦截器之后的静态类型事实，后端实际字段是否与 `T`
 * 一致仍需以 `src/types/api.ts` 的契约为准。
 */
export function request<T = unknown>(config: AxiosRequestConfig): Promise<T> {
  // Axios 1.19 wraps the custom response generic in a conditional type that
  // TypeScript cannot reduce for arbitrary T. Runtime behavior is still fixed
  // by the response interceptor above and locked by request.spec.ts.
  return service<T, T>(config).then((result) => result as T)
}

// 默认导出保留原始 axios 实例：仅用于需要实例级能力的场景
// （如集成测试改写 defaults.baseURL / defaults.adapter），业务请求一律走上面的 request()。
export default service
