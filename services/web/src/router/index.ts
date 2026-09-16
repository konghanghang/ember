import { createRouter, createWebHistory } from 'vue-router'
import type { RouteLocationNormalized, Router, RouteRecordRaw } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'
import { resetAllStores } from '@/store/reset'

const adminRouteMeta = { role: 'admin' } as const

export const routes: RouteRecordRaw[] = [
    {
      path: '/',
      name: 'home',
      component: () => import('../views/HomeView.vue'),
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('../views/user/RegisterView.vue'),
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('../views/ForgotPasswordView.vue'),
    },

    // Unified Console Routes
    {
      path: '/console',
      component: () => import('../views/console/Layout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          redirect: '/console/dashboard',
        },
        {
          path: 'dashboard',
          name: 'console-dashboard',
          component: () => import('../views/console/DashboardView.vue'),
        },
        {
          path: 'account',
          name: 'console-account',
          component: () => import('../views/console/AccountCenterView.vue'),
        },
        {
          path: 'p115',
          name: 'console-p115',
          meta: { role: 'user' },
          redirect: '/console/account',
        },
        {
          path: 'profile-analytics',
          name: 'console-profile-analytics',
          component: () => import('../views/console/ProfileAnalyticsView.vue'),
        },
        {
          path: 'subscriptions',
          name: 'console-subscriptions',
          component: () => import('../views/console/SubscriptionsView.vue'),
        },
        {
          path: 'rankings',
          name: 'console-rankings',
          component: () => import('../views/console/RankingsView.vue'),
        },
        {
          path: 'library',
          redirect: '/console/dashboard',
        },
        {
          path: 'tv-calendar',
          name: 'console-tv-calendar',
          component: () => import('../views/console/TVCalendarView.vue'),
        },
        {
          path: 'subscriptions/new',
          name: 'console-subscriptions-new',
          component: () => import('../views/console/NewSubscriptionView.vue'),
        },
        {
          path: 'renewal',
          name: 'console-renewal',
          component: () => import('../views/console/RenewalCenterView.vue'),
        },
        {
          path: 'pricing',
          redirect: '/console/renewal',
        },
        {
          path: 'users',
          name: 'console-users',
          meta: adminRouteMeta,
          component: () => import('../views/admin/UserCenterView.vue'),
        },
        {
          path: 'playback',
          name: 'console-playback',
          meta: adminRouteMeta,
          component: () => import('../views/admin/PlaybackCenterView.vue'),
        },
        {
          path: 'playback/users/:id',
          name: 'console-user-profile',
          meta: adminRouteMeta,
          component: () => import('../views/admin/UserPlaybackProfileView.vue'),
        },
        {
          path: 'user-profiles',
          redirect: (to) => ({
            path: '/console/playback',
            query: {
              ...to.query,
              tab: 'profiles'
            }
          })
        },
        {
          path: 'user-profiles/:id',
          redirect: (to) => ({
            path: `/console/playback/users/${String(to.params.id ?? '')}`,
            query: to.query
          })
        },
        {
          path: 'users/:id/profile',
          redirect: (to) => ({
            path: `/console/playback/users/${String(to.params.id ?? '')}`,
            query: to.query
          })
        },
        {
          path: 'redemptions',
          name: 'console-redemptions',
          meta: adminRouteMeta,
          component: () => import('../views/admin/RedemptionCenterView.vue'),
        },
        {
          path: 'redemption-codes',
          redirect: (to) => ({
            path: '/console/redemptions',
            query: {
              ...to.query,
              tab: 'codes'
            }
          })
        },
        {
          path: 'redemption-history',
          redirect: (to) => ({
            path: '/console/redemptions',
            query: {
              ...to.query,
              tab: 'history'
            }
          })
        },
        {
          path: 'settings',
          name: 'console-settings',
          meta: adminRouteMeta,
          component: () => import('../views/admin/SettingsView.vue'),
        },
        {
          path: 'p115-accounts',
          name: 'console-p115-accounts',
          meta: adminRouteMeta,
          component: () => import('../views/admin/P115AccountsView.vue'),
        },
        {
          path: 'sessions',
          name: 'console-sessions',
          meta: adminRouteMeta,
          component: () => import('../views/admin/SessionsView.vue'),
        },
        {
          path: 'playback-history',
          redirect: (to) => ({
            path: '/console/playback',
            query: {
              ...to.query,
              tab: 'history'
            }
          })
        },
        {
          path: 'media-quality',
          name: 'console-media-quality',
          meta: adminRouteMeta,
          component: () => import('../views/admin/MediaQualityView.vue'),
        },
        {
          path: 'media-gaps',
          name: 'console-media-gaps',
          meta: adminRouteMeta,
          component: () => import('../views/admin/MediaGapsView.vue'),
        },
        {
          path: 'devices',
          name: 'console-devices',
          meta: adminRouteMeta,
          component: () => import('../views/admin/DevicesView.vue'),
        },
        {
          path: 'billing',
          name: 'console-billing',
          meta: adminRouteMeta,
          component: () => import('../views/admin/PaymentCenterView.vue'),
        },
        {
          path: 'plans',
          redirect: (to) => ({
            path: '/console/billing',
            query: {
              ...to.query,
              tab: 'plans'
            }
          })
        },
        {
          path: 'payments',
          redirect: (to) => ({
            path: '/console/billing',
            query: {
              ...to.query,
              tab: 'payments'
            }
          })
        },
      ],
    },

    // Legacy redirects
    { path: '/admin/users', redirect: '/console/users' },
    { path: '/admin/redemption-codes', redirect: '/console/redemptions?tab=codes' },
    { path: '/admin/redemption-history', redirect: '/console/redemptions?tab=history' },
    { path: '/admin/plans', redirect: '/console/billing?tab=plans' },
    { path: '/admin/payments', redirect: '/console/billing?tab=payments' },
    { path: '/admin/subscriptions', redirect: '/console/subscriptions' },
    { path: '/admin/settings', redirect: '/console/settings' },
    { path: '/admin/p115-accounts', redirect: '/console/p115-accounts' },
    { path: '/user/dashboard', redirect: '/console/dashboard' },
    { path: '/user/subscriptions/new', redirect: '/console/subscriptions/new' },
    { path: '/user/subscriptions', redirect: '/console/subscriptions' },
    { path: '/admin/:pathMatch(.*)*', redirect: '/console/dashboard' },
    { path: '/user/:pathMatch(.*)*', redirect: '/console/dashboard' },

    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('../views/NotFoundView.vue'),
    },
]

/**
 * 判断 profile 加载失败是否代表认证失效。
 *
 * 生产环境 request.ts 的 401 回调会同步清场；守卫仍保留这个判断，保证 fake API
 * 测试和直接调用 userStore.fetchProfile 的路径也不会把 401 当成可恢复故障。
 */
function isProfileAuthFailure(error: unknown) {
  return typeof error === 'object'
    && error !== null
    && 'response' in error
    && (error as { response?: { status?: number } }).response?.status === 401
}

/**
 * 判断当前 URL 是否请求登录页恢复展示。
 *
 * `recovery=profile` 只是 UI 展示开关，不是“已验证 profile 失败”的鉴权事实；
 * 已有 profile 的会话会继续走普通已登录跳转，避免伪造 query 后误报服务器故障。
 */
function isLoginRecoveryRoute(to: Pick<RouteLocationNormalized, 'name' | 'query'>) {
  return to.name === 'login' && to.query.recovery === 'profile'
}

/**
 * 安装认证导航守卫。
 *
 * 生产路由和测试路由共用同一套守卫，测试可以用 memory history 复现真实导航。
 * `/profile` 的 401 会清理身份；网络错误和 5xx 保留 token，进入登录页恢复态并等待重试。
 */
export function installAuthGuards(targetRouter: Router) {
  targetRouter.beforeEach(async (to, _from, next) => {
    const authStore = useAuthStore()
    const userStore = useUserStore()
    authStore.restoreAuth()
    const requiresAuth = to.matched.some((record) => record.meta.requiresAuth === true)
    const requiredRole = to.matched.find((record) => typeof record.meta.role === 'string')?.meta.role

    if (isLoginRecoveryRoute(to) && authStore.isAuthenticated && !userStore.profile) {
      next()
      return
    }

    if (to.name === 'login' && authStore.isAuthenticated) {
      next({ name: authStore.passwordResetRequired ? 'console-account' : 'console-dashboard' })
      return
    }

    if (requiresAuth) {
      if (!authStore.isAuthenticated) {
        next({ name: 'login', query: { redirect: to.fullPath } })
        return
      }

      if (!userStore.profile) {
        try {
          await userStore.fetchProfile()
        } catch (error) {
          if (isProfileAuthFailure(error)) {
            resetAllStores()
            next({ name: 'login', query: { redirect: to.fullPath } })
            return
          }

          next({ name: 'login', query: { redirect: to.fullPath, recovery: 'profile' } })
          return
        }
      }
      // profile 已就位时无需额外同步：role/passwordResetRequired 已从 userStore.profile 派生。

      if (requiredRole && requiredRole !== authStore.role) {
        ElMessage.warning('当前账号无权访问该页面')
        next({ name: 'console-dashboard' })
        return
      }

      const resetRequired = authStore.passwordResetRequired || userStore.profile?.passwordResetRequired === true
      if (resetRequired && to.name !== 'console-account') {
        ElMessage.warning('当前账号必须先修改密码')
        next({ name: 'console-account' })
        return
      }
    }

    next()
  })
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

installAuthGuards(router)

export default router
