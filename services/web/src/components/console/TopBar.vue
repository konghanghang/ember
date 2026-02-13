<script setup lang="ts">
import { useRouter, useRoute } from 'vue-router'
import { computed } from 'vue'
import { useAuthStore } from '@/store/auth'
import { ElMessage } from 'element-plus'
import {
  Fold,
  Expand,
  Bell,
  Search,
  ChatDotRound,
  TopRight,
  UserFilled,
  SwitchButton
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const props = defineProps<{
  collapsed: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle-sidebar'): void
}>()

const handleLogout = async () => {
  await authStore.logout()
  ElMessage.success('已登出')
  router.push('/login')
}

const currentRouteName = computed(() => {
  switch (route.name) {
    case 'console-dashboard': return '概览'
    case 'console-subscriptions': return '我的订阅'
    case 'console-users': return '用户管理'
    case 'console-redemption-codes': return '兑换码'
    case 'console-settings': return '系统设置'
    default: return '控制台'
  }
})

const breadcrumbs = computed(() => {
  const paths = [{ name: '首页', path: '/' }, { name: '控制台', path: '/console' }]
  if (route.name !== 'console-dashboard') {
    paths.push({ name: currentRouteName.value, path: route.path })
  }
  return paths
})

const communityLinks = [
  {
    title: '交流群组',
    url: 'https://t.me/NextNewEP_emby_chat'
  },
  {
    title: '入库通知频道',
    url: 'https://t.me/NextNewEP'
  }
]
</script>

<template>
  <header class="h-16 bg-white border-b border-gray-100 flex items-center justify-between px-6 sticky top-0 z-10 backdrop-blur-sm bg-white/90">
    <!-- Left: Toggle & Breadcrumbs -->
    <div class="flex items-center gap-4">
      <button 
        @click="$emit('toggle-sidebar')"
        class="p-2 rounded-lg hover:bg-gray-100 text-gray-500 transition-colors lg:hidden"
      >
        <el-icon :size="20">
          <component :is="collapsed ? Expand : Fold" />
        </el-icon>
      </button>

      <div class="flex items-center gap-2 text-sm text-gray-500">
        <span class="font-medium text-gray-900">{{ currentRouteName }}</span>
      </div>
    </div>

    <!-- Right: Actions -->
    <div class="flex items-center gap-4">
      <!-- Search (Placeholder) -->
      <div class="hidden md:flex items-center bg-gray-50 rounded-full px-4 py-1.5 border border-transparent focus-within:border-ember/30 focus-within:bg-white transition-all">
        <el-icon :size="16" class="text-gray-400"><Search /></el-icon>
        <input 
          type="text" 
          placeholder="搜索..." 
          class="bg-transparent border-none outline-none text-sm ml-2 w-48 text-gray-600 placeholder-gray-400"
        />
      </div>

      <!-- Notifications (Placeholder) -->
      <button class="p-2 rounded-full hover:bg-gray-100 text-gray-400 hover:text-ember transition-colors relative">
        <el-icon :size="20"><Bell /></el-icon>
        <span class="absolute top-2 right-2 w-2 h-2 bg-red-500 rounded-full border-2 border-white"></span>
      </button>

      <el-dropdown trigger="click" placement="bottom-end">
        <button class="p-2 rounded-full hover:bg-gray-100 text-gray-400 hover:text-ember transition-colors">
          <el-icon :size="20"><ChatDotRound /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu class="w-52">
            <el-dropdown-item v-for="item in communityLinks" :key="item.url">
              <a
                :href="item.url"
                target="_blank"
                rel="noopener noreferrer"
                class="w-full flex items-center justify-between gap-2 text-sm text-gray-700"
              >
                <span>{{ item.title }}</span>
                <el-icon :size="12" class="text-gray-400"><TopRight /></el-icon>
              </a>
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <div class="h-6 w-px bg-gray-200 mx-1"></div>

      <!-- User Dropdown -->
      <el-dropdown trigger="click">
        <div class="flex items-center gap-3 cursor-pointer hover:bg-gray-50 p-1.5 rounded-lg transition-colors">
          <div class="w-8 h-8 rounded-full bg-gradient-to-br from-ember to-orange-500 flex items-center justify-center text-white shadow-sm ring-2 ring-white">
            <el-icon :size="14"><UserFilled /></el-icon>
          </div>
          <div class="hidden md:block text-left">
            <p class="text-xs font-semibold text-gray-700 leading-tight">{{ authStore.username }}</p>
            <p class="text-[10px] text-gray-400 uppercase tracking-wider">{{ authStore.role }}</p>
          </div>
        </div>
        <template #dropdown>
          <el-dropdown-menu class="w-48">
            <div class="px-4 py-3 border-b border-gray-100 mb-1">
              <p class="text-sm font-medium text-gray-900">Signed in as</p>
              <p class="text-xs text-gray-500 truncate">{{ authStore.username }}</p>
            </div>
            <el-dropdown-item :icon="UserFilled" @click="router.push('/console/dashboard')">个人中心</el-dropdown-item>
            <el-dropdown-item :icon="SwitchButton" divided @click="handleLogout">退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </header>
</template>
