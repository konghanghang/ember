<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowDown,
  Expand,
  Fold,
  Setting,
  SwitchButton
} from '@element-plus/icons-vue'
import DefaultAvatar from '@/components/common/DefaultAvatar.vue'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const userStore = useUserStore()

defineProps<{
  collapsed: boolean
}>()

defineEmits<{
  (e: 'toggle-sidebar'): void
}>()

const routeMeta: Record<string, { title: string }> = {
  'console-dashboard': { title: '概览' },
  'console-account': { title: '账号中心' },
  'console-profile-analytics': { title: '我的画像' },
  'console-subscriptions': { title: '订阅管理' },
  'console-subscriptions-new': { title: '新建订阅' },
  'console-rankings': { title: '播放排行榜' },
  'console-tv-calendar': { title: '追剧日历' },
  'console-renewal': { title: '续费中心' },
  'console-users': { title: '用户管理' },
  'console-user-profile': { title: '用户画像' },
  'console-playback': { title: '播放分析' },
  'console-redemptions': { title: '兑换中心' },
  'console-settings': { title: '系统设置' },
  'console-sessions': { title: '活跃会话' },
  'console-media-quality': { title: '媒体质量' },
  'console-devices': { title: '设备管理' },
  'console-billing': { title: '支付中心' }
}

const currentMeta = computed(() => {
  const name = String(route.name)
  if (name === 'console-playback') {
    const tab = route.query.tab
    if (tab === 'history') {
      return { title: '播放分析 · 播放历史' }
    }
    return { title: '播放分析 · 用户画像' }
  }
  return routeMeta[name] ?? { title: '控制台' }
})

const profile = computed(() => userStore.profile)
const displayName = computed(() => profile.value?.username || '当前用户')
const displayEmail = computed(() => profile.value?.email || '未设置联系邮箱')
const isTelegramBound = computed(() => !!profile.value?.telegramId)
const isExpired = computed(() => {
  if (!profile.value?.expiresAt) return false
  return new Date(profile.value.expiresAt) < new Date()
})
const membershipLabel = computed(() => {
  if (authStore.isAdmin) return '管理员'
  return isExpired.value ? '已过期' : '有效会员'
})

const handleLogout = async () => {
  await authStore.logout()
  userStore.clearUserData()
  ElMessage.success('已登出')
  router.push('/login')
}
</script>

<template>
  <header class="sticky top-0 z-10 flex h-[72px] items-center justify-between border-b border-gray-200 bg-white/90 px-4 backdrop-blur sm:px-6">
    <div class="flex min-w-0 items-center gap-4">
      <button
        aria-label="切换侧边栏"
        class="rounded-xl p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 lg:hidden cursor-pointer"
        @click="$emit('toggle-sidebar')"
      >
        <el-icon :size="20">
          <component :is="collapsed ? Expand : Fold" />
        </el-icon>
      </button>

      <div class="min-w-0">
        <p class="truncate text-lg font-semibold text-gray-900">{{ currentMeta.title }}</p>
      </div>
    </div>

    <div class="flex items-center gap-3">
      <el-dropdown trigger="click" placement="bottom-end">
        <button
          aria-label="打开账号菜单"
          class="group inline-flex h-11 items-center gap-2 rounded-full border border-gray-200 bg-white/90 py-1 pl-1.5 pr-2.5 text-left transition-colors hover:border-ember/30 hover:bg-white focus:outline-none focus:ring-4 focus:ring-ember/10 cursor-pointer"
        >
          <DefaultAvatar :name="displayName" size="sm" shape="full" />
          <span class="hidden min-w-0 max-w-[9rem] items-center gap-2 sm:inline-flex">
            <span class="truncate text-sm font-semibold text-gray-900">{{ displayName }}</span>
            <span
              role="img"
              :aria-label="membershipLabel"
              class="h-2 w-2 shrink-0 rounded-full"
              :class="isExpired ? 'bg-red-500' : 'bg-emerald-500'"
            ></span>
          </span>
          <el-icon class="text-gray-400 transition-colors group-hover:text-ember" :size="14">
            <ArrowDown />
          </el-icon>
        </button>

        <template #dropdown>
          <div class="w-[min(20rem,calc(100vw-2rem))] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-xl shadow-gray-900/10">
            <div class="px-4 py-4">
              <div class="flex items-start gap-3">
                <DefaultAvatar :name="displayName" size="lg" shape="2xl" />
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold text-gray-900">{{ displayName }}</p>
                  <p class="mt-1 truncate text-xs text-gray-500">{{ displayEmail }}</p>
                </div>
              </div>

              <div class="mt-4 grid grid-cols-2 gap-2">
                <span
                  class="inline-flex items-center justify-center gap-1.5 rounded-xl px-2.5 py-2 text-[11px] font-medium"
                  :class="isExpired ? 'bg-red-50 text-red-700' : 'bg-emerald-50 text-emerald-700'"
                >
                  <span class="h-2 w-2 rounded-full" :class="isExpired ? 'bg-red-500' : 'bg-emerald-500'"></span>
                  {{ membershipLabel }}
                </span>
                <span
                  class="inline-flex items-center justify-center gap-1.5 rounded-xl px-2.5 py-2 text-[11px] font-medium"
                  :class="isTelegramBound ? 'bg-sky-50 text-sky-700' : 'bg-gray-100 text-gray-600'"
                >
                  <span class="h-2 w-2 rounded-full" :class="isTelegramBound ? 'bg-sky-500' : 'bg-gray-400'"></span>
                  {{ isTelegramBound ? 'Telegram 已绑定' : 'Telegram 未绑定' }}
                </span>
              </div>
            </div>

            <div class="border-t border-gray-100 p-2">
              <button
                class="flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-left transition-colors hover:bg-gray-50 cursor-pointer"
                @click="router.push('/console/account')"
              >
                <div>
                  <p class="text-sm font-semibold text-gray-900">账号中心</p>
                  <p class="mt-0.5 text-xs text-gray-500">资料、安全、绑定</p>
                </div>
                <el-icon class="text-gray-400"><Setting /></el-icon>
              </button>

              <button
                class="mt-1 flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-left text-sm font-semibold text-gray-700 transition-colors hover:bg-red-50 hover:text-red-600 cursor-pointer"
                @click="handleLogout"
              >
                <span>退出登录</span>
                <el-icon><SwitchButton /></el-icon>
              </button>
            </div>
          </div>
        </template>
      </el-dropdown>
    </div>
  </header>
</template>
