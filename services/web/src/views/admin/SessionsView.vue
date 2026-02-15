<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { DataLine, RefreshRight, VideoPlay, Monitor } from '@element-plus/icons-vue'
import { getActiveSessions } from '@/api/admin'
import type { ActiveNowPlayingItem, ActiveSession } from '@/types/api'

const sessions = ref<ActiveSession[]>([])
const loading = ref(true)
const refreshing = ref(false)
const autoRefresh = ref(true)
const lastRefreshedAt = ref('')

const REFRESH_INTERVAL = 10000 // 10 秒
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
  // 剧集：seriesName S01E02
  if (item.seriesName && item.parentIndexNumber != null && item.indexNumber != null) {
    const season = String(item.parentIndexNumber).padStart(2, '0')
    const episode = String(item.indexNumber).padStart(2, '0')
    return `${item.seriesName} S${season}E${episode}`
  }
  // 电影：Name (Year)
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
  if (method === 'DirectPlay') return 'bg-green-100 text-green-700 border border-green-200'
  if (method === 'DirectStream') return 'bg-blue-100 text-blue-700 border border-blue-200'
  if (method === 'Transcode') return 'bg-orange-100 text-orange-700 border border-orange-200'
  return 'bg-gray-100 text-gray-700 border border-gray-200'
}

function playMethodLabel(method: string | undefined): string {
  if (method === 'DirectPlay') return 'DirectPlay'
  if (method === 'DirectStream') return 'DirectStream'
  if (method === 'Transcode') return 'Transcode'
  return method || 'Unknown'
}

function updateLastRefreshedAt() {
  const d = new Date()
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  lastRefreshedAt.value = `${hh}:${mm}:${ss}`
}

async function fetchSessions(opts?: { silent?: boolean }) {
  const silent = opts?.silent === true

  if (silent) {
    refreshing.value = true
  } else {
    loading.value = true
  }

  try {
    const res = await getActiveSessions({ silent })
    sessions.value = res.data || []
    updateLastRefreshedAt()
  } catch (err) {
    sessions.value = []
    if (!silent) {
      ElMessage.error('获取活跃会话失败')
    }
    // eslint-disable-next-line no-console
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
    <!-- Header -->
    <div class="flex items-start md:items-center justify-between gap-4 flex-col md:flex-row">
      <div class="flex items-center gap-3">
        <div class="p-2 rounded-xl bg-ember/10 text-ember">
          <el-icon :size="20"><DataLine /></el-icon>
        </div>
        <div>
          <h1 class="text-2xl font-bold text-gray-900">活跃会话</h1>
          <p class="text-sm text-gray-500 mt-1">
            <span>当前有 {{ sessionCount }} 个会话正在播放</span>
            <span v-if="lastRefreshedAt" class="ml-2 text-gray-400">刷新于 {{ lastRefreshedAt }}</span>
          </p>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <div class="flex items-center gap-2 bg-white border border-gray-100 rounded-xl px-3 py-2 shadow-sm">
          <span class="text-xs text-gray-500 font-semibold">自动刷新</span>
          <el-switch v-model="autoRefresh" />
          <span class="text-xs text-gray-400">10s</span>
        </div>

        <button
          type="button"
          @click="fetchSessions()"
          :disabled="loading || refreshing"
          class="px-4 py-2 rounded-xl bg-gray-900 text-white hover:bg-black transition-colors font-bold shadow-sm disabled:opacity-60 disabled:cursor-not-allowed flex items-center gap-2"
          title="刷新"
        >
          <span v-if="refreshing" class="animate-spin w-4 h-4 border-2 border-white/30 border-t-white rounded-full"></span>
          <el-icon v-else :size="18"><RefreshRight /></el-icon>
          刷新
        </button>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="!loading && sessions.length === 0"
      class="bg-white rounded-2xl border border-gray-100 shadow-sm p-10 text-center"
    >
      <div class="mx-auto w-12 h-12 rounded-2xl bg-gray-50 flex items-center justify-center text-gray-400">
        <el-icon :size="22"><VideoPlay /></el-icon>
      </div>
      <h3 class="mt-4 text-lg font-bold text-gray-900">当前没有活跃的播放会话</h3>
      <p class="mt-2 text-sm text-gray-500">当有人在 Emby 上开始播放时，这里会在 10 秒内自动更新。</p>
    </div>

    <!-- Session Cards -->
    <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <div
        v-for="s in sessions"
        :key="s.id"
        class="bg-white rounded-2xl border border-gray-100 shadow-sm hover:shadow-md hover:border-gray-200 transition-all p-6"
      >
        <div class="flex items-start justify-between gap-4">
          <div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-500">用户</span>
              <span class="font-bold text-gray-900">{{ s.userName || 'Unknown' }}</span>
            </div>
          </div>

          <span
            class="text-[11px] px-2.5 py-1 rounded-full font-bold tracking-wide"
            :class="playMethodClass(s.playState?.playMethod)"
          >
            {{ playMethodLabel(s.playState?.playMethod) }}
          </span>
        </div>

        <div class="mt-4">
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2 min-w-0">
              <div class="w-9 h-9 rounded-xl bg-gray-50 flex items-center justify-center text-gray-500 shrink-0">
                <el-icon :size="18"><Monitor /></el-icon>
              </div>
              <div class="min-w-0">
                <p class="font-bold text-gray-900 truncate">
                  {{ s.nowPlayingItem ? formatMediaName(s.nowPlayingItem) : 'Unknown Media' }}
                </p>
                <p class="text-xs text-gray-500 mt-0.5 truncate">
                  {{ s.client || 'Unknown Client' }} · {{ s.deviceName || 'Unknown Device' }}
                </p>
              </div>
            </div>

            <span
              v-if="s.playState?.isPaused"
              class="text-[11px] px-2 py-0.5 rounded-full bg-gray-100 text-gray-600 border border-gray-200 font-semibold shrink-0"
            >⏸ 已暂停</span>
          </div>

          <div class="mt-5">
            <div class="h-2.5 bg-gray-100 rounded-full overflow-hidden">
              <div
                class="h-full bg-gradient-to-r from-ember to-orange-500 rounded-full transition-all duration-700 ease-out"
                :style="{ width: `${progressPercent(s)}%` }"
              ></div>
            </div>
            <div class="mt-2 flex items-center justify-between text-xs text-gray-500 font-mono">
              <span>
                {{ formatDuration(ticksToSeconds(s.playState?.positionTicks || 0)) }}
              </span>
              <span>
                {{ formatDuration(ticksToSeconds(s.nowPlayingItem?.runTimeTicks || 0)) }}
              </span>
            </div>
          </div>

          <div class="mt-4 text-xs text-gray-500">
            <span class="font-semibold text-gray-600">IP</span>
            <span class="ml-1 font-mono">{{ s.remoteEndpoint || '-' }}</span>
            <span class="mx-2 text-gray-300">|</span>
            <span class="font-semibold text-gray-600">版本</span>
            <span class="ml-1 font-mono">{{ s.applicationVersion || '-' }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
