<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Bell,
  ChatDotRound,
  CircleCloseFilled,
  CopyDocument,
  Film,
  Monitor,
  Reading,
  Setting,
  VideoPlay
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { useConsoleStore } from '@/store/console'
import { useUserStore } from '@/store/user'
import { getEmbyConfig, getMediaStats } from '@/api/console'
import type { ConsoleAccountLinkIcon, MediaStats, UserInfo } from '@/types/api'

const authStore = useAuthStore()
const consoleStore = useConsoleStore()
const userStore = useUserStore()
const router = useRouter()

const emptyUser: UserInfo = {
  id: '',
  username: '',
  role: 'user',
  email: '',
  embyId: '',
  expiresAt: '',
  embyDisabled: false,
  isActive: false,
  createdAt: ''
}

const user = computed(() => userStore.profile ?? emptyUser)
const embyUrl = ref('')
const stats = ref<MediaStats>({ MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 })
const loading = ref(false)

const resourceIconMap: Record<ConsoleAccountLinkIcon, Component> = {
  notify: Bell,
  group: ChatDotRound,
  wiki: Reading
}

const isExpired = computed(() => {
  if (!user.value.expiresAt) return false
  return new Date(user.value.expiresAt) < new Date()
})

const daysLeft = computed(() => {
  if (!user.value.expiresAt) return null
  const ms = new Date(user.value.expiresAt).getTime() - Date.now()
  return Math.ceil(ms / (24 * 60 * 60 * 1000))
})

const showLockedServerState = computed(() => !authStore.isAdmin && isExpired.value)
const isLifetimeMember = computed(() => !user.value.expiresAt)
const membershipStatusLabel = computed(() => {
  if (isExpired.value) return '已过期'
  if (isLifetimeMember.value) return '永久有效'
  return '有效'
})
const membershipStatusTextClass = computed(() => {
  if (isExpired.value) return 'text-red-300'
  if (isLifetimeMember.value) return 'text-sky-200'
  return 'text-emerald-300'
})
const membershipStatusDescription = computed(() => {
  if (isExpired.value) return '服务已暂停，请尽快续费。'
  if (isLifetimeMember.value) return '无到期限制，可长期使用。'
  return `剩余 ${daysLeft.value} 天可用。`
})

const fetchOverview = async () => {
  if (!userStore.profile) return

  loading.value = true
  try {
    if (showLockedServerState.value) {
      embyUrl.value = ''
      stats.value = { MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 }
      return
    }

    const [configRes, statsRes] = await Promise.all([getEmbyConfig(), getMediaStats()])
    if (configRes.success) embyUrl.value = configRes.url
    if (statsRes.success) stats.value = statsRes.data
  } finally {
    loading.value = false
  }
}

const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('复制成功')
  } catch {
    ElMessage.error('复制失败')
  }
}

watch(
  () => [userStore.profile?.id, userStore.profile?.expiresAt, authStore.isAdmin],
  async ([profileID]) => {
    if (!profileID) return
    await fetchOverview()
  },
  { immediate: true }
)
</script>

<template>
  <div class="space-y-8 animate-fade-in" v-loading="loading">
    <div class="relative overflow-hidden rounded-3xl bg-slate-900 text-white shadow-xl">
      <div class="absolute inset-0 bg-[radial-gradient(circle_at_top_right,_rgba(249,115,22,0.22),_transparent_35%),radial-gradient(circle_at_bottom_left,_rgba(59,130,246,0.18),_transparent_30%)]"></div>
      <div class="relative p-8 md:p-10">
        <div class="flex flex-col gap-8 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-start gap-5">
            <div class="flex h-20 w-20 items-center justify-center rounded-3xl bg-white/10 text-3xl font-bold text-white ring-1 ring-white/10">
              {{ (user.username || 'U').charAt(0).toUpperCase() }}
            </div>
            <div>
              <div class="flex flex-wrap items-center gap-3">
                <h1 class="text-3xl font-semibold tracking-tight">{{ user.username || '当前用户' }}</h1>
                <span class="rounded-full border border-white/15 bg-white/10 px-3 py-1 text-xs font-semibold text-white/85">
                  {{ authStore.isAdmin ? '管理员账号' : '用户账号' }}
                </span>
              </div>
              <div class="mt-4 flex flex-wrap gap-3 text-sm text-slate-300">
                <span
                  class="rounded-full px-3 py-1 font-medium"
                  :class="user.expiresAt ? 'bg-white/10 text-slate-200' : 'bg-emerald-400/20 text-emerald-200 ring-1 ring-emerald-300/20'"
                >
                  到期时间：{{ user.expiresAt ? new Date(user.expiresAt).toLocaleDateString() : '永久有效' }}
                </span>
                <span
                  class="rounded-full px-3 py-1 font-medium"
                  :class="user.telegramId ? 'bg-sky-400/20 text-sky-200 ring-1 ring-sky-300/20' : 'bg-white/10 text-slate-200'"
                >
                  Telegram：{{ user.telegramId ? '已绑定' : '未绑定' }}
                </span>
              </div>
            </div>
          </div>

          <div class="min-w-[16rem] rounded-3xl border border-white/10 bg-white/5 p-5 backdrop-blur">
            <p class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">会员状态</p>
            <div class="mt-3 flex items-end justify-between gap-4">
              <div>
                <p class="text-2xl font-semibold" :class="membershipStatusTextClass">
                  {{ membershipStatusLabel }}
                </p>
                <p class="mt-1 text-sm text-slate-300">
                  {{ membershipStatusDescription }}
                </p>
              </div>
              <button
                v-if="!authStore.isAdmin"
                class="rounded-2xl bg-white px-4 py-2 text-sm font-semibold text-slate-900 transition-colors hover:bg-slate-100 cursor-pointer"
                @click="router.push('/console/renewal')"
              >
                {{ isExpired ? '立即续费' : '去续费' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="isExpired && !authStore.isAdmin"
      class="flex items-center gap-4 rounded-2xl border border-red-100 bg-red-50 p-4 text-red-800"
    >
      <div class="flex h-10 w-10 items-center justify-center rounded-full bg-red-100">
        <el-icon :size="20"><CircleCloseFilled /></el-icon>
      </div>
      <div class="flex-1">
        <h3 class="text-sm font-semibold">服务已暂停</h3>
        <p class="mt-1 text-xs text-red-600">您的订阅已过期。请前往续费中心恢复 Emby 访问权限。</p>
      </div>
      <button
        class="text-sm font-semibold text-red-700 underline transition-colors hover:text-red-900 cursor-pointer"
        @click="router.push('/console/renewal')"
      >
        去续费
      </button>
    </div>

    <div class="grid gap-6 md:grid-cols-3">
      <div class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm transition-colors hover:border-purple-200">
        <div class="flex items-center gap-4">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-purple-50 text-purple-600">
            <el-icon :size="24"><Film /></el-icon>
          </div>
          <div>
            <p class="text-3xl font-semibold text-slate-900">{{ stats.MovieCount }}</p>
            <p class="text-sm text-slate-500">电影收藏</p>
          </div>
        </div>
      </div>

      <div class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm transition-colors hover:border-emerald-200">
        <div class="flex items-center gap-4">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-600">
            <el-icon :size="24"><VideoPlay /></el-icon>
          </div>
          <div>
            <p class="text-3xl font-semibold text-slate-900">{{ stats.SeriesCount }}</p>
            <p class="text-sm text-slate-500">剧集收藏</p>
          </div>
        </div>
      </div>

      <div class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm transition-colors hover:border-sky-200">
        <div class="flex items-center gap-4">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-sky-50 text-sky-600">
            <el-icon :size="24"><Monitor /></el-icon>
          </div>
          <div>
            <p class="text-3xl font-semibold text-slate-900">{{ stats.EpisodeCount }}</p>
            <p class="text-sm text-slate-500">总集数</p>
          </div>
        </div>
      </div>
    </div>

    <section v-if="consoleStore.accountLinks.length > 0" class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
      <h2 class="text-lg font-semibold text-slate-900">帮助与资源</h2>

      <div class="mt-5 grid gap-4 md:grid-cols-3">
        <a
          v-for="item in consoleStore.accountLinks"
          :key="item.key"
          :href="item.url"
          target="_blank"
          rel="noreferrer"
          class="flex items-start gap-3 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 no-underline transition-colors hover:border-slate-300 hover:bg-white"
        >
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-white text-slate-700 shadow-sm">
            <el-icon :size="18">
              <component :is="resourceIconMap[item.icon]" />
            </el-icon>
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-semibold text-slate-900">{{ item.title }}</p>
            <p class="mt-1 text-xs leading-5 text-slate-500">{{ item.description }}</p>
          </div>
        </a>
      </div>
    </section>

    <div class="grid gap-8 xl:grid-cols-[1.1fr_0.9fr]">
      <section class="rounded-3xl border border-slate-200 bg-white shadow-sm">
        <div class="border-b border-slate-100 px-6 py-5">
          <h2 class="text-lg font-semibold text-slate-900">服务器连接</h2>
        </div>

        <div class="p-6">
          <div v-if="embyUrl && (!isExpired || authStore.isAdmin)" class="space-y-4">
            <div class="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">服务器地址</p>
              <div class="mt-3 flex items-center gap-2">
                <code class="min-w-0 flex-1 truncate rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-700">
                  {{ embyUrl }}
                </code>
                <button
                  aria-label="复制服务器地址"
                  class="rounded-2xl p-3 text-slate-400 transition-colors hover:bg-white hover:text-ember cursor-pointer"
                  @click="copyToClipboard(embyUrl)"
                >
                  <el-icon><CopyDocument /></el-icon>
                </button>
              </div>
            </div>
            <p class="text-sm leading-6 text-slate-500">
              使用此地址在 Emby 客户端登录，控制台与 Emby 客户端共用同一套账号密码。
            </p>
          </div>

          <div
            v-else-if="showLockedServerState"
            class="rounded-2xl border border-dashed border-red-200 bg-red-50 px-6 py-10 text-center text-red-700"
          >
            <el-icon :size="36" class="text-red-300"><Monitor /></el-icon>
            <p class="mt-3 text-sm font-semibold">服务器访问已锁定</p>
            <p class="mt-2 text-xs text-red-600">当前账号已过期，请先续费后再恢复 Emby 访问权限。</p>
          </div>

          <div v-else class="rounded-2xl border border-dashed border-slate-200 bg-slate-50 px-6 py-10 text-center text-slate-500">
            <el-icon :size="36" class="text-slate-300"><Monitor /></el-icon>
            <p class="mt-3 text-sm">当前未提供服务器入口。</p>
          </div>
        </div>
      </section>

      <div class="space-y-8">
        <section class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
          <div class="border-b border-slate-100 pb-5">
            <h2 class="text-lg font-semibold text-slate-900">快捷操作</h2>
          </div>

          <div class="mt-5 space-y-3">
            <button
              class="group flex w-full items-center gap-4 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 text-left transition-colors hover:border-slate-300 hover:bg-white cursor-pointer"
              @click="router.push('/console/account')"
            >
              <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-white text-slate-700 shadow-sm transition-colors group-hover:text-ember">
                <el-icon :size="20"><Setting /></el-icon>
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-sm font-semibold text-slate-900">账号中心</p>
                <p class="mt-1 text-xs text-slate-500">编辑邮箱、密码和 Telegram 绑定</p>
              </div>
            </button>
            <button
              class="group flex w-full items-center gap-4 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 text-left transition-colors hover:border-slate-300 hover:bg-white cursor-pointer"
              @click="router.push('/console/subscriptions')"
            >
              <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-white text-slate-700 shadow-sm transition-colors group-hover:text-ember">
                <el-icon :size="20"><VideoPlay /></el-icon>
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-sm font-semibold text-slate-900">订阅管理</p>
                <p class="mt-1 text-xs text-slate-500">查看当前求片记录和审核状态</p>
              </div>
            </button>
            <button
              v-if="!authStore.isAdmin"
              class="group flex w-full items-center gap-4 rounded-2xl border border-ember/20 bg-ember/5 px-4 py-4 text-left transition-colors hover:border-ember/30 hover:bg-white cursor-pointer"
              @click="router.push('/console/renewal')"
            >
              <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-white text-ember shadow-sm">
                <el-icon :size="20"><Monitor /></el-icon>
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-sm font-semibold text-slate-900">续费中心</p>
                <p class="mt-1 text-xs text-slate-500">购买方案或兑换续期码</p>
              </div>
              <span class="rounded-full bg-white px-2.5 py-1 text-xs font-semibold text-ember shadow-sm">
                {{ isExpired ? '优先处理' : '快捷入口' }}
              </span>
            </button>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.4s ease-out forwards;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(8px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
