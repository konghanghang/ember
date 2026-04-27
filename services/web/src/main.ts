import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { ElMessage } from 'element-plus'
import App from './App.vue'
import { registerElementPlus } from './plugins/element-plus'
import router from './router'
import { useAuthStore } from './store/auth'
import './style.css'
import './assets/base.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)
registerElementPlus(app)

const authStore = useAuthStore(pinia)
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
