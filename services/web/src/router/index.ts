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
          path: 'subscriptions/new',
          name: 'console-subscriptions-new',
          component: () => import('../views/console/NewSubscriptionView.vue'),
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
          path: 'settings',
          name: 'console-settings',
          meta: { role: 'admin' },
          component: () => import('../views/admin/SettingsView.vue'),
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
