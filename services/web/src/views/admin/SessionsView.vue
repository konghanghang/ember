<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RefreshRight, VideoPlay, Monitor } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { formatTimeWithSeconds } from '@/utils/date'
import { getActiveSessions } from '@/api/admin'
import type { ActiveNowPlayingItem, ActiveSession } from '@/types/api'

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const sessions = ref<ActiveSession[]>([])
const loading = ref(true)
const refreshing = ref(false)
const autoRefresh = ref(true)
const lastRefreshedAt = ref('')
const loadError = ref('')

const REFRESH_INTERVAL = 10000
let timer: ReturnType<typeof setInterval> | null = null

const sessionCount = computed(() => sessions.value.length)

function ticksToSeconds(ticks: number): number {
  if (!Number.isFinite(ticks) || ticks <= 0) return 0
  return Math.floor(ticks / 10000000)
}

function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) return '0:00'
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = Math.floor(totalSeconds % 60)
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
  }
  return `${minutes}:${String(seconds).padStart(2, '0')}`
}

function formatMediaName(item: ActiveNowPlayingItem): string {
  if (item.seriesName && item.parentIndexNumber != null && item.indexNumber != null) {
    const season = String(item.parentIndexNumber).padStart(2, '0')
    const episode = String(item.indexNumber).padStart(2, '0')
    return `${item.seriesName} S${season}E${episode}`
  }
  const year = item.productionYear ? ` (${item.productionYear})` : ''
  return `${item.name}${year}`
}

function progressPercent(session: ActiveSession): number {
  const total = session.nowPlayingItem?.runTimeTicks || 0
  const pos = session.playState?.positionTicks || 0
  if (!Number.isFinite(total) || total <= 0) return 0
  if (!Number.isFinite(pos) || pos <= 0) return 0
  const pct = (pos / total) * 100
  if (!Number.isFinite(pct)) return 0
  return Math.max(0, Math.min(100, pct))
}

function playMethodClass(method: string | undefined): string {
  if (method === 'DirectPlay') return 'border border-emerald-200 bg-emerald-50 text-emerald-700'
  if (method === 'DirectStream') return 'border border-sky-200 bg-sky-50 text-sky-700'
  if (method === 'Transcode') return 'border border-amber-200 bg-amber-50 text-amber-700'
  return 'border border-gray-200 bg-gray-100 text-gray-700'
}

function playMethodLabel(method: string | undefined): string {
  if (method === 'DirectPlay') return '直接播放'
  if (method === 'DirectStream') return '直接串流'
  if (method === 'Transcode') return '转码'
  return method || '未知方式'
}

function userDisplayName(session: ActiveSession): string {
  return session.userName || '未知用户'
}

function mediaDisplayName(session: ActiveSession): string {
  return session.nowPlayingItem ? formatMediaName(session.nowPlayingItem) : '未知媒体'
}

function clientDisplayName(session: ActiveSession): string {
  return session.client || '未知客户端'
}

function deviceDisplayName(session: ActiveSession): string {
  return session.deviceName || '未知设备'
}

function progressAriaLabel(session: ActiveSession): string {
  return `${mediaDisplayName(session)} 播放进度 ${Math.round(progressPercent(session))}%`
}

/** 最近刷新时间精确到秒，让用户能判断轮询是否仍在推进。 */
function updateLastRefreshedAt() {
  lastRefreshedAt.value = formatTimeWithSeconds(new Date())
}

async function fetchSessions(opts?: { silent?: boolean }) {
  const silent = opts?.silent === true

  if (silent) {
    refreshing.value = true
  } else {
    loading.value = true
  }

  try {
    const res = await getActiveSessions({ silent: true })
    sessions.value = res.data || []
    loadError.value = ''
    updateLastRefreshedAt()
  } catch (err) {
    loadError.value = silent ? '自动刷新失败，当前展示的是上一次成功获取的数据。' : '获取活跃会话失败，请稍后重试。'
    console.error(err)
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  if (autoRefresh.value) {
    timer = setInterval(() => {
      fetchSessions({ silent: true })
    }, REFRESH_INTERVAL)
  }
}

function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(autoRefresh, (val) => {
  if (val) startAutoRefresh()
  else stopAutoRefresh()
})

onMounted(async () => {
  await fetchSessions()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
})
</script>

<template>
  <div class="space-y-6 animate-fade-in" v-loading="loading">
    <EmberPageHeaderCard v-if="!embedded" title="活跃会话" description="查看当前 Emby 播放会话与刷新状态。">
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-normal text-gray-500">
          {{ sessionCount }} 个会话
        </span>
      </template>

      <template #actions>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 shadow-sm">
            <span class="text-xs font-semibold text-gray-500">自动刷新</span>
            <el-switch v-model="autoRefresh" aria-label="切换活跃会话自动刷新" />
            <span class="text-xs text-gray-400">10s</span>
          </div>

          <button
            type="button"
            @click="fetchSessions()"
            :disabled="loading || refreshing"
            class="inline-flex cursor-pointer items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2 font-semibold text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            aria-label="刷新活跃会话列表"
          >
            <span v-if="refreshing" class="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-ember"></span>
            <el-icon v-else :size="18"><RefreshRight /></el-icon>
            刷新
          </button>
        </div>
      </template>

      <div class="mt-4 flex flex-wrap items-center gap-2 text-xs text-gray-500">
        <span v-if="lastRefreshedAt" class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-gray-500">
          最近刷新 {{ lastRefreshedAt }}
        </span>
        <span
          v-if="loadError"
          class="inline-flex items-center rounded-full bg-red-50 px-2.5 py-1 text-red-600"
        >
          {{ loadError }}
        </span>
      </div>
    </EmberPageHeaderCard>

    <div v-else class="flex flex-wrap items-center justify-end gap-3">
      <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-500">{{ sessionCount }} 个会话</span>
      <div class="flex items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2">
        <span class="text-xs font-semibold text-gray-500">自动刷新</span>
        <el-switch v-model="autoRefresh" aria-label="切换活跃会话自动刷新" />
      </div>
      <button type="button" @click="fetchSessions()" :disabled="loading || refreshing" class="inline-flex cursor-pointer items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-semibold text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60">刷新</button>
    </div>

    <EmberEmptyStateCard
      v-if="!loading && loadError && sessions.length === 0"
      :icon="RefreshRight"
      tone="danger"
      title="活跃会话加载失败"
      description="当前无法获取 Emby 活跃会话，请稍后重试。"
    >
      <template #actions>
        <button
          type="button"
          @click="fetchSessions()"
          class="rounded-xl border border-red-200 bg-white px-4 py-2 text-sm font-semibold text-red-700 transition-colors hover:bg-red-50"
        >
          重新获取
        </button>
      </template>
    </EmberEmptyStateCard>

    <EmberEmptyStateCard
      v-else-if="!loading && sessions.length === 0"
      :icon="VideoPlay"
      title="当前没有活跃的播放会话"
      description="当有人在 Emby 上开始播放时，这里会在 10 秒内自动更新。"
    />

    <div v-else class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <article
        v-for="session in sessions"
        :key="session.id"
        class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm transition-all hover:border-gray-200 hover:shadow-md"
      >
        <div class="flex items-start justify-between gap-4">
          <div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-500">用户</span>
              <span class="font-bold text-gray-900">{{ userDisplayName(session) }}</span>
            </div>
          </div>

          <span
            class="rounded-full px-2.5 py-1 text-[11px] font-bold tracking-wide"
            :class="playMethodClass(session.playState?.playMethod)"
          >
            {{ playMethodLabel(session.playState?.playMethod) }}
          </span>
        </div>

        <div class="mt-4">
          <div class="flex items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2">
              <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gray-50 text-gray-500">
                <el-icon :size="18"><Monitor /></el-icon>
              </div>
              <div class="min-w-0">
                <p class="truncate font-bold text-gray-900">
                  {{ mediaDisplayName(session) }}
                </p>
                <p class="mt-0.5 truncate text-xs text-gray-500">
                  {{ clientDisplayName(session) }} · {{ deviceDisplayName(session) }}
                </p>
              </div>
            </div>

            <span
              v-if="session.playState?.isPaused"
              class="shrink-0 rounded-full border border-gray-200 bg-gray-100 px-2 py-0.5 text-[11px] font-semibold text-gray-600"
            >
              已暂停
            </span>
          </div>

          <div class="mt-5">
            <div
              class="h-2.5 overflow-hidden rounded-full bg-gray-100"
              role="progressbar"
              :aria-label="progressAriaLabel(session)"
              :aria-valuenow="Math.round(progressPercent(session))"
              aria-valuemin="0"
              aria-valuemax="100"
            >
              <div
                class="h-full rounded-full bg-gradient-to-r from-ember to-orange-500 transition-all duration-700 ease-out"
                :style="{ width: `${progressPercent(session)}%` }"
              ></div>
            </div>
            <div class="mt-2 flex items-center justify-between font-mono text-xs text-gray-500">
              <span>{{ formatDuration(ticksToSeconds(session.playState?.positionTicks || 0)) }}</span>
              <span>{{ formatDuration(ticksToSeconds(session.nowPlayingItem?.runTimeTicks || 0)) }}</span>
            </div>
          </div>

          <div class="mt-4 text-xs text-gray-500">
            <span class="font-semibold text-gray-600">IP</span>
            <span class="ml-1 font-mono">{{ session.remoteEndpoint || '-' }}</span>
            <span class="mx-2 text-gray-300">|</span>
            <span class="font-semibold text-gray-600">版本</span>
            <span class="ml-1 font-mono">{{ session.applicationVersion || '-' }}</span>
          </div>
        </div>
      </article>
    </div>
  </div>
</template>
