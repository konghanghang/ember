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
</script>

<template>
  <header class="h-16 bg-white border-b border-gray-100 flex items-center justify-between px-6 sticky top-0 z-10 backdrop-blur-sm bg-white/90">
    <!-- Left: Toggle & Breadcrumbs -->
    <div class="flex items-center gap-4">
      <button 
        @click="$emit('toggle-sidebar')"
        aria-label="切换侧边栏"
        class="p-2 rounded-lg hover:bg-gray-100 text-gray-500 transition-colors lg:hidden cursor-pointer"
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

      <!-- Community Dropdown -->
      <el-dropdown trigger="click" placement="bottom-end">
        <div class="flex items-center gap-2 cursor-pointer hover:bg-gray-50 p-2 rounded-lg transition-colors group">
          <div class="p-1.5 rounded-full bg-red-50 text-ember group-hover:bg-red-100 transition-colors relative">
            <svg class="w-[18px] h-[18px]" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>
            <span class="absolute -top-1 -right-1 flex h-2.5 w-2.5">
              <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span>
              <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500"></span>
            </span>
          </div>
          <span class="hidden md:block text-sm font-medium text-gray-600 group-hover:text-gray-900">社区</span>
        </div>
        <template #dropdown>
          <div class="w-64 p-2 bg-white rounded-xl shadow-lg border border-gray-100">
            <div class="px-2 py-1.5 mb-1">
              <p class="text-xs font-semibold text-gray-400 uppercase tracking-wider">Community</p>
            </div>
            
            <a href="https://t.me/NextNewEP" target="_blank" class="block no-underline">
              <div class="flex items-start gap-3 p-2 hover:bg-gray-50 rounded-lg transition-colors group/item cursor-pointer">
                <div class="p-2 bg-red-50 text-red-500 rounded-lg group-hover/item:bg-red-100 transition-colors">
                  <el-icon :size="18"><Bell /></el-icon>
                </div>
                <div>
                  <p class="text-sm font-medium text-gray-900 flex items-center gap-2 m-0">
                    通知频道
                    <span class="text-[10px] bg-red-100 text-red-600 px-1.5 py-0.5 rounded-full font-bold">New</span>
                  </p>
                  <p class="text-xs text-gray-500 mt-0.5 m-0 leading-tight">获取最新入库通知</p>
                </div>
              </div>
            </a>

            <a href="https://t.me/NextNewEP_emby_chat" target="_blank" class="block mt-1 no-underline">
              <div class="flex items-start gap-3 p-2 hover:bg-gray-50 rounded-lg transition-colors group/item cursor-pointer">
                <div class="p-2 bg-red-50 text-ember rounded-lg group-hover/item:bg-red-100 transition-colors">
                  <svg class="w-[18px] h-[18px]" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>
                </div>
                <div>
                  <p class="text-sm font-medium text-gray-900 m-0">交流群组</p>
                  <p class="text-xs text-gray-500 mt-0.5 m-0 leading-tight">加入社区讨论与求助</p>
                </div>
              </div>
            </a>
          </div>
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
