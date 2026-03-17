<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Bell,
  ChatDotRound,
  Expand,
  Fold,
  Reading,
  Setting,
  SwitchButton
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { useConsoleStore } from '@/store/console'
import { useUserStore } from '@/store/user'
import type { ConsoleAccountLinkIcon } from '@/types/api'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const consoleStore = useConsoleStore()
const userStore = useUserStore()

defineProps<{
  collapsed: boolean
}>()

defineEmits<{
  (e: 'toggle-sidebar'): void
}>()

const routeMeta: Record<string, { title: string; description: string }> = {
  'console-dashboard': {
    title: '概览',
    description: '查看会员状态、服务器入口和媒体统计'
  },
  'console-account': {
    title: '账号中心',
    description: '集中管理资料、安全设置与常用资源'
  },
  'console-subscriptions': {
    title: '订阅管理',
    description: '维护求片请求与订阅记录'
  },
  'console-subscriptions-new': {
    title: '新建订阅',
    description: '提交新的影片或剧集订阅需求'
  },
  'console-rankings': {
    title: '播放排行榜',
    description: '查看近期热门内容与排行快照'
  },
  'console-library': {
    title: '媒体库',
    description: '浏览当前片库和基础媒体信息'
  },
  'console-tv-calendar': {
    title: '追剧日历',
    description: '管理追更节奏与播出提醒'
  },
  'console-renewal': {
    title: '续费中心',
    description: '购买方案或兑换续期码'
  },
  'console-users': {
    title: '用户管理',
    description: '维护用户账号与状态'
  },
  'console-redemptions': {
    title: '兑换中心',
    description: '统一管理兑换码池和兑换记录'
  },
  'console-settings': {
    title: '系统设置',
    description: '管理系统配置与运行参数'
  },
  'console-sessions': {
    title: '活跃会话',
    description: '查看在线设备与登录会话'
  },
  'console-playback-history': {
    title: '播放历史',
    description: '审计近期播放记录'
  },
  'console-media-quality': {
    title: '媒体质量',
    description: '盘点片库质量和转码信息'
  },
  'console-devices': {
    title: '设备管理',
    description: '查看设备状态与管控记录'
  },
  'console-billing': {
    title: '支付中心',
    description: '统一管理付费方案和支付记录'
  }
}

const resourceIconMap: Record<ConsoleAccountLinkIcon, Component> = {
  notify: Bell,
  group: ChatDotRound,
  wiki: Reading
}

const currentMeta = computed(() => routeMeta[String(route.name)] ?? {
  title: '控制台',
  description: '统一查看账号与业务信息'
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
  <header class="sticky top-0 z-10 flex h-[72px] items-center justify-between border-b border-slate-200 bg-white/90 px-4 backdrop-blur sm:px-6">
    <div class="flex min-w-0 items-center gap-4">
      <button
        aria-label="切换侧边栏"
        class="rounded-xl p-2 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 lg:hidden cursor-pointer"
        @click="$emit('toggle-sidebar')"
      >
        <el-icon :size="20">
          <component :is="collapsed ? Expand : Fold" />
        </el-icon>
      </button>

      <div class="min-w-0">
        <p class="truncate text-lg font-semibold text-slate-900">{{ currentMeta.title }}</p>
        <p class="hidden truncate text-sm text-slate-500 md:block">{{ currentMeta.description }}</p>
      </div>
    </div>

    <div class="flex items-center gap-3">
      <el-dropdown trigger="click" placement="bottom-end">
        <button class="flex items-center gap-3 rounded-2xl border border-slate-200 bg-white px-3 py-2 text-left shadow-sm transition-colors hover:border-slate-300 hover:bg-slate-50 cursor-pointer">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-slate-900 to-slate-700 text-sm font-semibold text-white">
            {{ displayName.charAt(0).toUpperCase() }}
          </div>
          <div class="hidden min-w-0 sm:block">
            <p class="truncate text-sm font-semibold text-slate-900">{{ displayName }}</p>
            <p class="truncate text-xs text-slate-500">{{ membershipLabel }}</p>
          </div>
        </button>

        <template #dropdown>
          <div class="w-[22rem] rounded-3xl border border-slate-200 bg-white p-3 shadow-xl">
            <div class="rounded-2xl bg-slate-50 px-4 py-4">
              <div class="flex items-start gap-3">
                <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-slate-900 to-slate-700 text-base font-semibold text-white">
                  {{ displayName.charAt(0).toUpperCase() }}
                </div>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold text-slate-900">{{ displayName }}</p>
                  <p class="mt-1 truncate text-xs text-slate-500">{{ displayEmail }}</p>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <span
                      class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium"
                      :class="isExpired ? 'bg-red-50 text-red-700' : 'bg-emerald-50 text-emerald-700'"
                    >
                      <span class="h-2 w-2 rounded-full" :class="isExpired ? 'bg-red-500' : 'bg-emerald-500'"></span>
                      {{ membershipLabel }}
                    </span>
                    <span
                      class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium"
                      :class="isTelegramBound ? 'bg-sky-50 text-sky-700' : 'bg-slate-200 text-slate-600'"
                    >
                      <span class="h-2 w-2 rounded-full" :class="isTelegramBound ? 'bg-sky-500' : 'bg-slate-400'"></span>
                      {{ isTelegramBound ? 'Telegram 已绑定' : 'Telegram 未绑定' }}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <div class="mt-3 grid gap-2 sm:grid-cols-2">
              <button
                class="flex items-center justify-between rounded-2xl border border-slate-200 px-4 py-3 text-left transition-colors hover:border-slate-300 hover:bg-slate-50 cursor-pointer"
                @click="router.push('/console/account')"
              >
                <div>
                  <p class="text-sm font-semibold text-slate-900">账号中心</p>
                  <p class="mt-1 text-xs text-slate-500">资料、安全、绑定</p>
                </div>
                <el-icon class="text-slate-400"><Setting /></el-icon>
              </button>
              <button
                v-if="!authStore.isAdmin"
                class="flex items-center justify-between rounded-2xl border border-ember/20 bg-ember/5 px-4 py-3 text-left transition-colors hover:border-ember/30 hover:bg-white cursor-pointer"
                @click="router.push('/console/renewal')"
              >
                <div>
                  <p class="text-sm font-semibold text-slate-900">续费中心</p>
                  <p class="mt-1 text-xs text-slate-500">方案购买与兑换码</p>
                </div>
                <span class="rounded-full bg-white px-2 py-1 text-[11px] font-semibold text-ember shadow-sm">
                  {{ isExpired ? '立即处理' : '快捷入口' }}
                </span>
              </button>
            </div>

            <div v-if="consoleStore.accountLinks.length > 0" class="mt-4 border-t border-slate-100 pt-4">
              <p class="px-1 text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">常用资源</p>
              <div class="mt-2 space-y-1">
                <a
                  v-for="item in consoleStore.accountLinks"
                  :key="item.key"
                  :href="item.href"
                  target="_blank"
                  rel="noreferrer"
                  class="flex items-start gap-3 rounded-2xl px-3 py-3 no-underline transition-colors hover:bg-slate-50"
                >
                  <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-slate-100 text-slate-700">
                    <el-icon :size="18">
                      <component :is="resourceIconMap[item.icon]" />
                    </el-icon>
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-slate-900">{{ item.title }}</p>
                    <p class="mt-1 text-xs leading-5 text-slate-500">{{ item.description }}</p>
                  </div>
                </a>
              </div>
            </div>

            <button
              class="mt-3 flex w-full items-center justify-center gap-2 rounded-2xl border border-slate-200 px-4 py-3 text-sm font-semibold text-slate-700 transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-600 cursor-pointer"
              @click="handleLogout"
            >
              <el-icon><SwitchButton /></el-icon>
              退出登录
            </button>
          </div>
        </template>
      </el-dropdown>
    </div>
  </header>
</template>
