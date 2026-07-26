import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { ElMessage } from 'element-plus'
import App from './App.vue'
import { registerElementPlus } from './plugins/element-plus'
import router from './router'
import { setupRequestInterceptors } from './api/request'
import { useAuthStore } from './store/auth'
import { useConsoleStore } from './store/console'
import { useUserStore } from './store/user'
import { resetAllStores } from './store/reset'
import './style.css'
import './assets/base.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)
registerElementPlus(app)

const authStore = useAuthStore(pinia)
const consoleStore = useConsoleStore(pinia)
const userStore = useUserStore(pinia)

// 计算未授权时的跳转目标：保留当前 fullPath 供登录后返回；
// 已在 /login（避免回环）或无当前路由时回退到 dashboard。
function getUnauthorizedRedirectTarget() {
  const currentRoute = router.currentRoute.value
  if (!currentRoute || currentRoute.path === '/login') {
    return '/console/dashboard'
  }

  return currentRoute.fullPath
}

/**
 * 装配 request 拦截器依赖（P2-6）。
 * store/router 引用集中在此持有；request.ts 仅持有函数引用，斩断模块级循环依赖。
 * 401 处理改为调用 resetAllStores 统一清场（P3：消灭 5 处重复 teardown）。
 */
setupRequestInterceptors({
  getToken: () => authStore.token,
  onUnauthorized: async () => {
    const redirect = getUnauthorizedRedirectTarget()
    resetAllStores()
    ElMessage.error('登录已过期，请重新登录')
    await router.push({
      path: '/login',
      query: { redirect }
    })
  }
})

// consoleStore/userStore 在 setupRequestInterceptors 闭包内通过 resetAllStores 间接使用；
// 显式列出引用以保留语义可见性，并防止 tree-shaking 在未来误删 store 装配。
void consoleStore
void userStore

authStore.restoreAuth()
authStore.initCrossTabSync((reason) => {
  const currentPath = router.currentRoute.value.path
  if (reason === 'signed-out') {
    if (currentPath !== '/login') {
      ElMessage.warning('已在其他窗口登出')
      void router.push({
        path: '/login',
        query: { redirect: router.currentRoute.value.fullPath }
      })
    }
    return
  }

  ElMessage.info('检测到其他窗口已切换账号，当前页面将同步登录态')
  void router.replace({ name: 'console-dashboard' })
})

app.mount('#app')
