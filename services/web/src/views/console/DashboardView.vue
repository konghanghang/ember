<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  CircleCloseFilled,
  CopyDocument,
  Film,
  Monitor,
  Setting,
  VideoPlay
} from '@element-plus/icons-vue'
import DefaultAvatar from '@/components/common/DefaultAvatar.vue'
import RecentLibrarySection from '@/components/console/RecentLibrarySection.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'
import { getMediaStats } from '@/api/console'
import type { MediaStats, UserInfo } from '@/types/api'

const authStore = useAuthStore()
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
const embyUrl = computed(() => userStore.embyUrl)
const stats = ref<MediaStats>({ MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 })
const loading = ref(false)

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
  if (isExpired.value) return 'text-red-600'
  if (isLifetimeMember.value) return 'text-sky-600'
  return 'text-emerald-600'
})
const membershipStatusDescription = computed(() => {
  if (isExpired.value) return '服务已暂停，请尽快续费。'
  if (isLifetimeMember.value) return '无到期限制，可长期使用。'
  return `剩余 ${daysLeft.value} 天可用。`
})
const canOpenEmby = computed(() => Boolean(embyUrl.value) && !showLockedServerState.value)

const fetchOverview = async () => {
  if (!userStore.profile) return

  loading.value = true
  try {
    if (showLockedServerState.value) {
      stats.value = { MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 }
      return
    }

    const [configResult, statsResult] = await Promise.allSettled([
      userStore.fetchEmbyConfig(),
      getMediaStats()
    ])

    if (configResult.status === 'rejected') {
      userStore.embyUrl = ''
    }

    if (statsResult.status === 'fulfilled' && statsResult.value.success) {
      stats.value = statsResult.value.data
    } else {
      stats.value = { MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 }
    }
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

const openEmby = () => {
  if (!canOpenEmby.value) return
  window.open(embyUrl.value, '_blank', 'noopener,noreferrer')
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
  <div class="space-y-6 animate-fade-in" v-loading="loading">
    <div class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
      <div class="p-8 md:p-10">
        <div class="flex flex-col gap-8 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-start gap-5">
            <DefaultAvatar :name="user.username" size="hero" shape="2xl" />
            <div>
              <div class="flex flex-wrap items-center gap-3">
                <h1 class="text-2xl font-bold tracking-tight text-gray-900">{{ user.username || '当前用户' }}</h1>
                <span class="rounded-full border border-gray-200 bg-gray-100 px-3 py-1 text-xs font-semibold text-gray-600">
                  {{ authStore.isAdmin ? '管理员账号' : '用户账号' }}
                </span>
              </div>
              <div class="mt-4 flex flex-wrap gap-3 text-sm text-gray-500">
                <span
                  class="rounded-full px-3 py-1 font-medium"
                  :class="isExpired
                    ? 'bg-red-50 text-red-700 ring-1 ring-red-200'
                    : user.expiresAt
                      ? 'bg-amber-50 text-amber-700 ring-1 ring-amber-200'
                      : 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200'"
                >
                  到期时间：{{ user.expiresAt ? new Date(user.expiresAt).toLocaleDateString() : '永久有效' }}
                </span>
                <span
                  class="rounded-full px-3 py-1 font-medium"
                  :class="user.telegramId
                    ? 'bg-sky-50 text-sky-700 ring-1 ring-sky-200'
                    : 'bg-gray-100 text-gray-500'"
                >
                  Telegram：{{ user.telegramId ? '已绑定' : '未绑定' }}
                </span>
              </div>
            </div>
          </div>

          <div class="min-w-[16rem] rounded-2xl border border-gray-100 bg-gray-50 p-5">
            <p class="text-xs font-semibold uppercase tracking-[0.2em] text-gray-400">会员状态</p>
            <div class="mt-3 flex items-end justify-between gap-4">
              <div>
                <p class="text-2xl font-semibold" :class="membershipStatusTextClass">
                  {{ membershipStatusLabel }}
                </p>
                <p class="mt-1 text-sm text-gray-500">
                  {{ membershipStatusDescription }}
                </p>
              </div>
              <button
                v-if="!authStore.isAdmin"
                class="btn-ember rounded-xl px-4 py-2 text-sm cursor-pointer"
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
      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm transition-colors hover:border-purple-200">
        <div class="flex items-center gap-4">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-purple-50 text-purple-600">
            <el-icon :size="24"><Film /></el-icon>
          </div>
          <div>
            <p class="text-3xl font-semibold text-gray-900">{{ stats.MovieCount }}</p>
            <p class="text-sm text-gray-500">电影收藏</p>
          </div>
        </div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm transition-colors hover:border-emerald-200">
        <div class="flex items-center gap-4">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-600">
            <el-icon :size="24"><VideoPlay /></el-icon>
          </div>
          <div>
            <p class="text-3xl font-semibold text-gray-900">{{ stats.SeriesCount }}</p>
            <p class="text-sm text-gray-500">剧集收藏</p>
          </div>
        </div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm transition-colors hover:border-sky-200">
        <div class="flex items-center gap-4">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-sky-50 text-sky-600">
            <el-icon :size="24"><Monitor /></el-icon>
          </div>
          <div>
            <p class="text-3xl font-semibold text-gray-900">{{ stats.EpisodeCount }}</p>
            <p class="text-sm text-gray-500">总集数</p>
          </div>
        </div>
      </div>
    </div>

    <div class="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
      <section class="rounded-2xl border border-gray-100 bg-white shadow-sm">
        <div class="border-b border-gray-100 px-6 py-5">
          <h2 class="text-lg font-semibold text-gray-900">Emby 入口</h2>
        </div>

        <div class="p-6">
          <div v-if="embyUrl && (!isExpired || authStore.isAdmin)" class="space-y-4">
            <div class="rounded-2xl border border-gray-100 bg-gray-50 p-4">
              <div class="space-y-3">
                <p class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">服务器地址</p>

                <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
                  <code class="min-w-0 flex-1 truncate rounded-2xl border border-gray-100 bg-white px-4 py-3 text-sm text-gray-700">
                    {{ embyUrl }}
                  </code>

                  <div class="flex items-center gap-2 lg:shrink-0">
                    <button
                      aria-label="复制服务器地址"
                      class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-400 transition-colors hover:bg-gray-50 hover:text-ember cursor-pointer"
                      @click="copyToClipboard(embyUrl)"
                    >
                      <el-icon><CopyDocument /></el-icon>
                    </button>

                    <button
                      type="button"
                      class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer lg:shrink-0"
                      @click="openEmby"
                    >
                      打开 Emby
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <p class="text-sm leading-6 text-gray-500">
              使用此地址在 Emby 客户端登录，控制台与 Emby 客户端共用同一套账号密码。
            </p>
          </div>

          <EmberEmptyStateCard
            v-else-if="showLockedServerState"
            :icon="Monitor"
            tone="danger"
            title="服务器访问已锁定"
            description="当前账号已过期，请先续费后再恢复 Emby 访问权限。"
          />

          <EmberEmptyStateCard
            v-else
            :icon="Monitor"
            title="当前未提供服务器入口"
            description="请联系管理员检查 Emby 连接配置。"
          />
        </div>
      </section>

      <div class="space-y-6">
        <section class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
          <div class="border-b border-gray-100 pb-5">
            <h2 class="text-lg font-semibold text-gray-900">快捷操作</h2>
          </div>

          <div class="mt-5 space-y-3">
            <button
              class="group flex w-full items-center gap-4 rounded-2xl border border-gray-100 bg-gray-50 px-4 py-4 text-left transition-colors hover:border-gray-200 hover:bg-white cursor-pointer"
              @click="router.push('/console/account')"
            >
              <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-white text-gray-700 shadow-sm transition-colors group-hover:text-ember">
                <el-icon :size="20"><Setting /></el-icon>
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-sm font-semibold text-gray-900">账号中心</p>
                <p class="mt-1 text-xs text-gray-500">编辑邮箱、密码和 Telegram 绑定</p>
              </div>
            </button>
            <button
              class="group flex w-full items-center gap-4 rounded-2xl border border-gray-100 bg-gray-50 px-4 py-4 text-left transition-colors hover:border-gray-200 hover:bg-white cursor-pointer"
              @click="router.push('/console/subscriptions')"
            >
              <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-white text-gray-700 shadow-sm transition-colors group-hover:text-ember">
                <el-icon :size="20"><VideoPlay /></el-icon>
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-sm font-semibold text-gray-900">订阅管理</p>
                <p class="mt-1 text-xs text-gray-500">查看当前求片记录和审核状态</p>
              </div>
            </button>
          </div>
        </section>
      </div>
    </div>

    <RecentLibrarySection :limit="20" />
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
