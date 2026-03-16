import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/store/auth'

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
          name: 'console-library',
          component: () => import('../views/console/LibraryView.vue'),
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
          meta: { role: 'admin' },
          component: () => import('../views/admin/UsersView.vue'),
        },
        {
          path: 'redemption-codes',
          name: 'console-redemption-codes',
          meta: { role: 'admin' },
          component: () => import('../views/admin/RedemptionCodesView.vue'),
        },
        {
          path: 'redemption-history',
          name: 'console-redemption-history',
          meta: { role: 'admin' },
          component: () => import('../views/admin/RedemptionHistoryView.vue'),
        },
        {
          path: 'settings',
          name: 'console-settings',
          meta: { role: 'admin' },
          component: () => import('../views/admin/SettingsView.vue'),
        },
        {
          path: 'sessions',
          name: 'console-sessions',
          meta: { role: 'admin' },
          component: () => import('../views/admin/SessionsView.vue'),
        },
        {
          path: 'playback-history',
          name: 'console-playback-history',
          meta: { role: 'admin' },
          component: () => import('../views/admin/PlaybackHistoryView.vue'),
        },
        {
          path: 'media-quality',
          name: 'console-media-quality',
          meta: { role: 'admin' },
          component: () => import('../views/admin/MediaQualityView.vue'),
        },
        {
          path: 'devices',
          name: 'console-devices',
          meta: { role: 'admin' },
          component: () => import('../views/admin/DevicesView.vue'),
        },
        {
          path: 'plans',
          name: 'console-plans',
          meta: { role: 'admin' },
          component: () => import('../views/admin/PlansView.vue'),
        },
        {
          path: 'payments',
          name: 'console-payments',
          meta: { role: 'admin' },
          component: () => import('../views/admin/PaymentsView.vue'),
        },
      ],
    },

    // Legacy redirects
    { path: '/admin/users', redirect: '/console/users' },
    { path: '/admin/redemption-codes', redirect: '/console/redemption-codes' },
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

  if (to.name === 'login' && authStore.isAuthenticated) {
    next({ name: 'console-dashboard' })
    return
  }

  if (to.meta.requiresAuth) {
    if (!authStore.isAuthenticated) {
      next({ name: 'login', query: { redirect: to.fullPath } })
      return
    }

    if (to.meta.role && to.meta.role !== authStore.role) {
      next({ name: 'console-dashboard' })
      return
    }
  }

  next()
})

export default router
