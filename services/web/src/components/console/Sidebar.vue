<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'
import {
  Odometer,
  VideoPlay,
  Trophy,
  Film,
  Setting,
  User,
  Ticket,
  Goods,
  ShoppingCart,
  Monitor,
  DataLine,
  Iphone,
  Calendar
} from '@element-plus/icons-vue'

const route = useRoute()
const authStore = useAuthStore()
const userStore = useUserStore()

const canAccessItem = (role?: string) => {
  if (!role) return true
  if (authStore.isAdmin) return true
  return authStore.role === role
}

const menuItems = computed(() => [
  {
    title: '概览',
    path: '/console/dashboard',
    icon: Odometer,
    role: 'user'
  },
  {
    title: '账号中心',
    path: '/console/account',
    icon: Setting,
    role: 'user'
  },
  {
    title: '订阅管理',
    path: '/console/subscriptions',
    icon: VideoPlay,
    role: 'user'
  },
  {
    title: '播放排行榜',
    path: '/console/rankings',
    icon: Trophy,
    role: 'user'
  },
  {
    title: '媒体库',
    path: '/console/library',
    icon: Film,
    role: 'user'
  },
  {
    title: '追剧日历',
    path: '/console/tv-calendar',
    icon: Calendar,
    role: 'user'
  },
  {
    title: '续费中心',
    path: '/console/renewal',
    icon: ShoppingCart
  },
  {
    title: '管理控制台',
    type: 'group',
    role: 'admin',
    children: [
      {
        title: '用户管理',
        path: '/console/users',
        icon: User,
        role: 'admin'
      },
      {
        title: '兑换中心',
        path: '/console/redemptions',
        icon: Ticket,
        role: 'admin'
      },
      {
        title: '支付中心',
        path: '/console/billing',
        icon: Goods,
        role: 'admin'
      },
      {
        title: '活跃会话',
        path: '/console/sessions',
        icon: DataLine,
        role: 'admin'
      },
      {
        title: '用户画像',
        path: '/console/user-profiles',
        icon: DataLine,
        role: 'admin'
      },
      {
        title: '播放历史',
        path: '/console/playback-history',
        icon: DataLine,
        role: 'admin'
      },
      {
        title: '媒体质量',
        path: '/console/media-quality',
        icon: Film,
        role: 'admin'
      },
      {
        title: '设备管理',
        path: '/console/devices',
        icon: Iphone,
        role: 'admin'
      },
      {
        title: '系统设置',
        path: '/console/settings',
        icon: Setting,
        role: 'admin'
      }
    ]
  }
])

const isActive = (path: string) => route.path === path || route.path.startsWith(`${path}/`)
const displayName = computed(() => userStore.profile?.username || '当前用户')
</script>

<template>
  <aside class="flex flex-col w-64 h-screen bg-white border-r border-gray-100 shadow-sm transition-all duration-300 z-20">
    <!-- Brand -->
    <div class="flex items-center justify-center h-16 px-6 border-b border-gray-50">
      <router-link to="/" class="flex items-center gap-2 text-ember font-bold text-xl hover:opacity-80 transition-opacity">
        <el-icon :size="24"><Monitor /></el-icon>
        <span>Ember Console</span>
      </router-link>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 overflow-y-auto py-6 px-3 space-y-1">
      <template v-for="item in menuItems" :key="item.title">
        <!-- Group Header -->
        <div v-if="item.type === 'group' && authStore.isAdmin" class="mt-8 mb-2 px-3">
          <h3 class="text-xs font-semibold text-gray-400 uppercase tracking-wider">
            {{ item.title }}
          </h3>
        </div>

        <!-- Group Children -->
        <template v-if="item.type === 'group' && authStore.isAdmin">
          <router-link
            v-for="child in item.children"
            :key="child.path"
            :to="child.path"
            class="flex items-center px-3 py-2.5 rounded-lg transition-colors group relative overflow-hidden"
            :class="[
              isActive(child.path) 
                ? 'bg-ember/10 text-ember font-medium' 
                : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
            ]"
          >
            <div 
              v-if="isActive(child.path)"
              class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-8 bg-ember rounded-r-full"
            ></div>
            <el-icon :size="18" :class="isActive(child.path) ? 'text-ember' : 'text-gray-400 group-hover:text-gray-500'">
              <component :is="child.icon" />
            </el-icon>
            <span class="ml-3">{{ child.title }}</span>
          </router-link>
        </template>

        <!-- Regular Item -->
        <router-link
          v-else-if="!item.type && canAccessItem(item.role)"
          :to="item.path"
          class="flex items-center px-3 py-2.5 rounded-lg transition-colors group relative overflow-hidden"
          :class="[
            isActive(item.path) 
              ? 'bg-ember/10 text-ember font-medium' 
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
          ]"
        >
          <div 
            v-if="isActive(item.path)"
            class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-8 bg-ember rounded-r-full"
          ></div>
          <el-icon :size="18" :class="isActive(item.path) ? 'text-ember' : 'text-gray-400 group-hover:text-gray-500'">
            <component :is="item.icon" />
          </el-icon>
          <span class="ml-3">{{ item.title }}</span>
        </router-link>
      </template>
    </nav>

    <!-- Footer / User Profile (simplified) -->
    <div class="p-4 border-t border-gray-50">
      <div class="flex items-center gap-3 rounded-xl border border-gray-100 bg-gray-50 px-3 py-2.5">
        <div class="flex h-8 w-8 items-center justify-center rounded-full bg-ember/10 text-ember ring-1 ring-ember/10">
          <span class="font-bold text-sm">{{ displayName.charAt(0).toUpperCase() || 'U' }}</span>
        </div>
        <div class="flex-1 min-w-0">
          <p class="text-sm font-medium text-gray-900 truncate">{{ displayName }}</p>
          <p class="text-xs text-gray-500 truncate">{{ authStore.role === 'admin' ? '管理员' : '普通用户' }}</p>
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* Custom scrollbar for nav */
nav::-webkit-scrollbar {
  width: 4px;
}
nav::-webkit-scrollbar-track {
  background: transparent;
}
nav::-webkit-scrollbar-thumb {
  background-color: #f1f1f1;
  border-radius: 4px;
}
nav:hover::-webkit-scrollbar-thumb {
  background-color: #d1d5db;
}
</style>
