<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  CircleCloseFilled,
  CopyDocument,
  Film,
  Monitor,
  VideoPlay
} from '@element-plus/icons-vue'
import DefaultAvatar from '@/components/common/DefaultAvatar.vue'
import RecentLibrarySection from '@/components/console/RecentLibrarySection.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import { formatDateOnly } from '@/utils/date'
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
const embyConfigured = computed(() => userStore.embyConfigured)
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
const membershipStatusMeta = computed(() => {
  if (isLifetimeMember.value) return '无到期限制'
  if (!user.value.expiresAt) return '未设置到期时间'
  return `到期于 ${formatDateOnly(user.value.expiresAt)}`
})

const membershipStatusHint = computed(() => {
  if (isExpired.value) return '服务已暂停，请前往续费中心恢复访问。'
  if (isLifetimeMember.value) return ''
  if (daysLeft.value === null) return ''
  return `剩余 ${daysLeft.value} 天`
})
const hasEmbyAccessUrl = computed(() => Boolean(embyUrl.value))

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
      // 仅在真实网络/异常时兜底；emby 未配置走 200 + configured:false，不再触发 reject。
      userStore.clearEmbyUrl()
      userStore.setEmbyConfigured(false)
    }

    if (
      statsResult.status === 'fulfilled' &&
      statsResult.value.success &&
      statsResult.value.data
    ) {
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

const openEmby = (url?: string) => {
  const target = url || embyUrl.value || ''
  if (!target || showLockedServerState.value) return
  window.open(target, '_blank', 'noopener,noreferrer')
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
    <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
      <div class="grid gap-6 p-6 md:p-8 xl:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <div class="space-y-6">
          <div class="flex items-start gap-5">
            <DefaultAvatar :name="user.username" size="hero" shape="2xl" />
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-3">
                <h1 class="truncate text-2xl font-bold tracking-tight text-gray-900">{{ user.username || '当前用户' }}</h1>
                <span class="rounded-full border border-gray-200 bg-gray-100 px-3 py-1 text-xs font-semibold text-gray-600">
                  {{ authStore.isAdmin ? '管理员账号' : '用户账号' }}
                </span>
              </div>

              <div class="mt-4 flex flex-wrap gap-3 text-sm text-gray-500">
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

          <div class="rounded-2xl border border-gray-100 bg-gray-50 p-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <p class="text-xs font-semibold uppercase tracking-[0.2em] text-gray-400">会员状态</p>
                <p class="mt-3 text-2xl font-semibold" :class="membershipStatusTextClass">
                  {{ membershipStatusLabel }}
                </p>
                <p class="mt-1 text-sm text-gray-500">
                  {{ membershipStatusMeta }}
                </p>
                <p
                  v-if="membershipStatusHint"
                  class="mt-2 text-xs font-medium"
                  :class="isExpired ? 'text-red-600' : 'text-gray-500'"
                >
                  {{ membershipStatusHint }}
                </p>
              </div>
              <button
                v-if="!authStore.isAdmin"
                class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer lg:shrink-0"
                @click="router.push('/console/renewal')"
              >
                {{ isExpired ? '立即续费' : '去续费' }}
              </button>
            </div>
          </div>
        </div>

        <div class="rounded-3xl border border-gray-100 bg-gradient-to-br from-gray-50 via-white to-white p-5 shadow-sm">
          <div class="flex flex-col gap-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 class="text-lg font-semibold text-gray-900">Emby 入口</h2>
                <p class="mt-1 text-sm text-gray-500">控制台与 Emby 客户端共用同一套账号密码。</p>
              </div>
              <span
                v-if="hasEmbyAccessUrl && !showLockedServerState"
                class="inline-flex w-fit items-center rounded-full bg-emerald-50 px-3 py-1 text-xs font-medium text-emerald-700"
              >
                当前可用
              </span>
            </div>

            <div v-if="hasEmbyAccessUrl && !showLockedServerState" class="space-y-3">
              <article
                class="rounded-2xl border border-ember/15 bg-white p-4 shadow-sm"
              >
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
                  <span class="inline-flex h-11 w-fit items-center rounded-full bg-ember/10 px-4 text-sm font-medium text-ember xl:shrink-0">
                    服务器地址
                  </span>

                  <code class="min-w-0 flex-1 truncate rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3 text-sm text-gray-700">
                    {{ embyUrl }}
                  </code>

                  <div class="flex items-center gap-2 xl:shrink-0">
                    <button
                      aria-label="复制 Emby 地址"
                      class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-400 transition-colors hover:bg-gray-50 hover:text-ember cursor-pointer"
                      @click="copyToClipboard(embyUrl)"
                    >
                      <el-icon><CopyDocument /></el-icon>
                    </button>

                    <button
                      type="button"
                      class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer"
                      @click="openEmby(embyUrl)"
                    >
                      打开
                    </button>
                  </div>
                </div>
              </article>
            </div>

            <EmberEmptyStateCard
              v-else-if="showLockedServerState"
              :icon="Monitor"
              tone="danger"
              title="服务器访问已锁定"
              description="当前账号已过期，请先续费后再恢复 Emby 访问权限。"
            />

            <EmberEmptyStateCard
              v-else-if="!embyConfigured"
              :icon="Monitor"
              tone="warning"
              title="系统尚未配置 Emby 服务器"
              :description="authStore.isAdmin
                ? '前往设置中心填写 Emby URL 与 API Key 后即可启用此入口。'
                : '系统正在配置中，请联系管理员完成 Emby 连接配置。'"
            >
              <template v-if="authStore.isAdmin" #actions>
                <button
                  type="button"
                  class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer"
                  @click="router.push('/admin/settings')"
                >
                  前往设置中心
                </button>
              </template>
            </EmberEmptyStateCard>

            <EmberEmptyStateCard
              v-else
              :icon="Monitor"
              title="当前未提供服务器入口"
              description="请联系管理员检查 Emby 连接配置。"
            />
          </div>
        </div>
      </div>
    </section>

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

    <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
      <div class="border-b border-gray-100 px-6 py-5">
        <div class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <h2 class="text-lg font-semibold text-gray-900">片库概览</h2>
          <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">
            实时摘要
          </span>
        </div>
      </div>

      <div class="grid gap-4 p-4 md:grid-cols-3 md:p-6">
        <article class="rounded-2xl border border-purple-100 bg-gradient-to-br from-purple-50 via-white to-white p-5 shadow-sm">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-gray-500">电影收藏</p>
              <p class="mt-3 text-3xl font-semibold text-gray-900">{{ stats.MovieCount }}</p>
            </div>
            <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-purple-100 text-purple-600 shadow-sm">
              <el-icon :size="24"><Film /></el-icon>
            </div>
          </div>
        </article>

        <article class="rounded-2xl border border-emerald-100 bg-gradient-to-br from-emerald-50 via-white to-white p-5 shadow-sm">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-gray-500">剧集收藏</p>
              <p class="mt-3 text-3xl font-semibold text-gray-900">{{ stats.SeriesCount }}</p>
            </div>
            <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-emerald-100 text-emerald-600 shadow-sm">
              <el-icon :size="24"><VideoPlay /></el-icon>
            </div>
          </div>
        </article>

        <article class="rounded-2xl border border-sky-100 bg-gradient-to-br from-sky-50 via-white to-white p-5 shadow-sm">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-gray-500">总集数</p>
              <p class="mt-3 text-3xl font-semibold text-gray-900">{{ stats.EpisodeCount }}</p>
            </div>
            <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-sky-100 text-sky-600 shadow-sm">
              <el-icon :size="24"><Monitor /></el-icon>
            </div>
          </div>
        </article>
      </div>
    </section>

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
