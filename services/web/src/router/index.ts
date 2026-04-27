import { createRouter, createWebHistory } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'

const adminRouteMeta = { role: 'admin' } as const

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
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
          component: () => import('../views/admin/UsersView.vue'),
        },
        {
          path: 'user-profiles',
          name: 'console-user-profiles',
          meta: adminRouteMeta,
          component: () => import('../views/admin/UserPlaybackProfilesView.vue'),
        },
        {
          path: 'user-profiles/:id',
          name: 'console-user-profile',
          meta: adminRouteMeta,
          component: () => import('../views/admin/UserPlaybackProfileView.vue'),
        },
        {
          path: 'users/:id/profile',
          redirect: (to) => ({
            path: `/console/user-profiles/${String(to.params.id ?? '')}`,
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
          path: 'sessions',
          name: 'console-sessions',
          meta: adminRouteMeta,
          component: () => import('../views/admin/SessionsView.vue'),
        },
        {
          path: 'playback-history',
          name: 'console-playback-history',
          meta: adminRouteMeta,
          component: () => import('../views/admin/PlaybackHistoryView.vue'),
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
  ],
})

// Navigation Guard
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  authStore.restoreAuth()
  const requiresAuth = to.matched.some((record) => record.meta.requiresAuth === true)
  const requiredRole = to.matched.find((record) => typeof record.meta.role === 'string')?.meta.role

  if (to.name === 'login' && authStore.isAuthenticated) {
    next({ name: 'console-dashboard' })
    return
  }

  if (requiresAuth) {
    if (!authStore.isAuthenticated) {
      next({ name: 'login', query: { redirect: to.fullPath } })
      return
    }

    if (requiredRole && requiredRole !== authStore.role) {
      ElMessage.warning('当前账号无权访问该页面')
      next({ name: 'console-dashboard' })
      return
    }
  }

  next()
})

export default router
