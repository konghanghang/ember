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
    // User Panel Routes
    {
      path: '/user',
      component: () => import('../views/user/Layout.vue'),
      meta: { requiresAuth: true, role: 'user' },
      children: [
        {
          path: 'dashboard',
          name: 'user-dashboard',
          component: () => import('../views/user/DashboardView.vue'),
        },
        {
          path: 'subscriptions',
          name: 'user-subscriptions',
          component: () => import('../views/user/SubscriptionsView.vue'),
        },
        {
          path: 'subscriptions/new',
          name: 'user-subscriptions-new',
          component: () => import('../views/user/NewSubscriptionView.vue'),
        },
      ],
    },
    // Admin Panel Routes
    {
      path: '/admin',
      component: () => import('../views/admin/Layout.vue'),
      meta: { requiresAuth: true, role: 'admin' },
      children: [
        {
          path: 'users',
          name: 'admin-users',
          component: () => import('../views/admin/UsersView.vue'),
        },
        {
          path: 'invites',
          name: 'admin-invites',
          component: () => import('../views/admin/InvitesView.vue'),
        },
        {
          path: 'subscriptions',
          name: 'admin-subscriptions',
          component: () => import('../views/admin/SubscriptionsView.vue'),
        },
        {
          path: 'settings',
          name: 'admin-settings',
          component: () => import('../views/admin/SettingsView.vue'),
        },
      ],
    },
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

  if (to.meta.requiresAuth) {
    if (!authStore.isAuthenticated) {
      next({ name: 'login', query: { redirect: to.fullPath } })
      return
    }

    if (to.meta.role && to.meta.role !== authStore.role) {
      next({ name: 'home' })
      return
    }
  }

  next()
})

export default router
