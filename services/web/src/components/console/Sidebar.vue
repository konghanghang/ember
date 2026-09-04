<script setup lang="ts">
import { computed } from 'vue'
import type { Component } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/store/auth'
import {
  Odometer,
  VideoPlay,
  Trophy,
  Film,
  Setting,
  User,
  Ticket,
  Goods,
  CollectionTag,
  ShoppingCart,
  Monitor,
  DataLine,
  Iphone,
  Calendar,
  FolderOpened,
  Key
} from '@element-plus/icons-vue'
import ProjectSourceLink from '@/components/common/ProjectSourceLink.vue'

// 菜单分两种条目：普通链接项与管理员分组（含子项）。
// 用可辨识联合建模，模板里 v-if="item.type === 'group'" 才能正确收窄出 children/path。
type RegularMenuItem = {
  title: string
  path: string
  icon: Component
  role?: string
  exactRole?: boolean
  type?: undefined
}

type GroupMenuItem = {
  title: string
  type: 'group'
  role: string
  children: RegularMenuItem[]
}

type MenuEntry = RegularMenuItem | GroupMenuItem

const route = useRoute()
const authStore = useAuthStore()

const canAccessItem = (item: RegularMenuItem) => {
  if (!item.role) return true
  if (item.exactRole) return authStore.role === item.role
  if (authStore.isAdmin) return true
  return authStore.role === item.role
}

const menuItems = computed<MenuEntry[]>(() => [
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
    title: '115 网盘',
    path: '/console/p115',
    icon: FolderOpened,
    role: 'user',
    exactRole: true
  },
  {
    title: '我的画像',
    path: '/console/profile-analytics',
    icon: DataLine,
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
        title: '用户分组',
        path: '/console/plan-groups',
        icon: CollectionTag,
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
        title: '播放分析',
        path: '/console/playback',
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
        title: '缺集管理',
        path: '/console/media-gaps',
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
        title: '115 账号',
        path: '/console/p115-accounts',
        icon: Key,
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
</script>

<template>
  <aside class="flex flex-col w-64 h-screen bg-white border-r border-gray-100 shadow-sm transition-all duration-300 z-20">
    <!-- Brand -->
    <div class="flex items-center justify-center h-[72px] px-6 border-b border-gray-50">
      <router-link to="/" class="flex items-center gap-2 text-ember font-bold text-xl hover:opacity-80 transition-opacity cursor-pointer">
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
            class="flex items-center px-3 py-2.5 rounded-lg transition-colors group relative overflow-hidden cursor-pointer"
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
          v-else-if="item.type !== 'group' && canAccessItem(item)"
          :to="item.path"
          class="flex items-center px-3 py-2.5 rounded-lg transition-colors group relative overflow-hidden cursor-pointer"
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

    <!-- Footer -->
    <div class="p-4 border-t border-gray-50">
      <div class="flex justify-center">
        <ProjectSourceLink />
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
