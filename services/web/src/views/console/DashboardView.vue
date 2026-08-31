<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  CopyDocument,
  Monitor
} from '@element-plus/icons-vue'
import RecentLibrarySection from '@/components/console/RecentLibrarySection.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import { formatDateOnly } from '@/utils/date'
import { copyToClipboard as copyTextToClipboard } from '@/utils/clipboard'
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
const emptyStats: MediaStats = { movieCount: 0, seriesCount: 0, episodeCount: 0 }
const stats = ref<MediaStats>({ ...emptyStats })
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
  // 过期态的提示统一交给 Emby 入口锁定空态 + 主卡续费按钮承担，主卡内不再重复。
  if (isExpired.value || isLifetimeMember.value) return ''
  if (daysLeft.value === null) return ''
  return `剩余 ${daysLeft.value} 天`
})
const hasEmbyAccessUrl = computed(() => Boolean(embyUrl.value))
const embyAccessLinks = computed(() => {
  const url = embyUrl.value?.trim()
  if (!url) return []
  return [
    {
      id: 'primary',
      label: '默认入口',
      url
    }
  ]
})

const fetchOverview = async () => {
  if (!userStore.profile) return

  loading.value = true
  try {
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
      stats.value = { ...emptyStats }
    }
  } finally {
    loading.value = false
  }
}

/** 复制结果提示归视图层，与全站既有口径一致。 */
const copyToClipboard = async (text: string) => {
  const ok = await copyTextToClipboard(text)
  if (ok) {
    ElMessage.success('复制成功')
  } else {
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
        <div class="rounded-2xl border border-gray-100 bg-gray-50 p-5">
          <div class="flex flex-col gap-5">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h1 class="text-lg font-semibold text-gray-900">服务状态</h1>
                <p class="mt-1 text-sm text-gray-500">会员有效期与 Emby 访问状态</p>
              </div>
              <span
                class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium"
                :class="isExpired ? 'bg-red-50 text-red-700' : isLifetimeMember ? 'bg-sky-50 text-sky-700' : 'bg-emerald-50 text-emerald-700'"
              >
                <span class="h-2 w-2 rounded-full" :class="isExpired ? 'bg-red-500' : isLifetimeMember ? 'bg-sky-500' : 'bg-emerald-500'"></span>
                {{ membershipStatusLabel }}
              </span>
            </div>

            <div>
              <p class="text-2xl font-semibold" :class="membershipStatusTextClass">
                {{ membershipStatusMeta }}
              </p>
              <p
                v-if="membershipStatusHint"
                class="mt-2 text-xs font-medium text-gray-500"
              >
                {{ membershipStatusHint }}
              </p>
            </div>

            <div v-if="!authStore.isAdmin" class="flex justify-start">
              <button
                class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer lg:shrink-0"
                @click="router.push('/console/renewal')"
              >
                {{ isExpired ? '立即续费' : '去续费' }}
              </button>
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-100 bg-gradient-to-br from-gray-50 via-white to-white p-5 shadow-sm">
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
                v-for="link in embyAccessLinks"
                :key="link.id"
                class="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm"
              >
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
                  <div class="min-w-0 flex-1">
                    <p class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">{{ link.label }}</p>
                    <code class="mt-2 block truncate rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3 text-sm text-gray-700">
                      {{ link.url }}
                    </code>
                  </div>

                  <div class="flex items-center gap-2 xl:shrink-0">
                    <button
                      aria-label="复制 Emby 地址"
                      class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-400 transition-colors hover:bg-gray-50 hover:text-ember cursor-pointer"
                      @click="copyToClipboard(link.url)"
                    >
                      <el-icon><CopyDocument /></el-icon>
                    </button>

                    <button
                      type="button"
                      class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer"
                      @click="openEmby(link.url)"
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

    <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
      <div class="border-b border-gray-100 px-6 py-5">
        <h2 class="text-lg font-semibold text-gray-900">片库概览</h2>
      </div>

      <div class="grid gap-4 p-4 md:grid-cols-3 md:p-6">
        <EmberMetricCard title="电影收藏" :value="stats.movieCount" />
        <EmberMetricCard title="剧集收藏" :value="stats.seriesCount" />
        <EmberMetricCard title="总集数" :value="stats.episodeCount" />
      </div>
    </section>

    <RecentLibrarySection :limit="20" />
  </div>
</template>
