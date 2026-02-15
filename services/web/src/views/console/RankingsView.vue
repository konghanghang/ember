<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Trophy, Film, VideoCamera, Calendar, Timer, VideoPlay } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { getLatestRanking, getRankingHistory } from '@/api/console'
import { previewRanking } from '@/api/admin'
import type { PlaybackRanking, RankingCategory, RankingPeriod } from '@/types/api'

const period = ref<RankingPeriod>('daily')
const loading = ref(false)

const movies = ref<PlaybackRanking[]>([])
const episodes = ref<PlaybackRanking[]>([])
const mode = ref<'latest' | 'preview' | 'history'>('latest')
const cutoffAt = ref('')
const selectedDate = ref('')

const authStore = useAuthStore()

const periodHint = computed(() => {
  return period.value === 'daily'
    ? '暂无播放数据，日榜将在每天 20:00 自动生成（阶段榜）'
    : '暂无播放数据，周榜将在每周日 20:30 自动生成（阶段榜）'
})

const rangeText = computed(() => {
  const sample = movies.value[0] || episodes.value[0]
  if (!sample) return ''

  const start = sample.periodStart?.slice(0, 10) || ''
  const end = sample.periodEnd?.slice(0, 10) || ''
  if (start !== '' && start === end) return `${start}`
  if (start !== '' && end !== '') return `${start} ~ ${end}`
  return ''
})

const rangeTextWithCutoff = computed(() => {
  if (!rangeText.value) return ''
  if (!cutoffAt.value) return rangeText.value
  return `${rangeText.value} 截至 ${cutoffAt.value}`
})

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0m'
  const totalMinutes = Math.floor(seconds / 60)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  if (hours >= 24) {
    const days = Math.floor(hours / 24)
    const restHours = hours % 24
    return `${days}天${restHours}h${minutes}m`
  }
  if (hours > 0) return `${hours}h${minutes}m`
  return `${minutes}m`
}

function rankBadgeClass(rank: number): string {
  if (rank === 1) return 'bg-amber-400'
  if (rank === 2) return 'bg-gray-400'
  if (rank === 3) return 'bg-amber-600'
  return ''
}

async function fetchCategory(category: RankingCategory): Promise<PlaybackRanking[]> {
  const res = await getLatestRanking(period.value, category)
  return res.data || []
}

function fakeISODate(dateStr: string, timeStr: string): string {
  // Make a parseable-ish timestamp string; UI only slices YYYY-MM-DD anyway.
  return `${dateStr}T${timeStr}:00`
}

async function fetchLatestAll() {
  loading.value = true
  mode.value = 'latest'
  cutoffAt.value = ''
  try {
    const [movieData, episodeData] = await Promise.all([
      fetchCategory('media_movie'),
      fetchCategory('media_episode')
    ])
    movies.value = movieData
    episodes.value = episodeData
  } catch (err) {
    movies.value = []
    episodes.value = []
    ElMessage.error('获取排行榜失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    loading.value = false
  }
}

function toYMD(d: Date): string {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function applyComputedList(
  source: 'preview' | 'history',
  res: {
    periodStart: string
    periodEnd: string
    cutoffAt: string
    snapshotAt?: string
    movies: { rank: number; itemName: string; playCount: number; duration: number }[]
    episodes: { rank: number; itemName: string; playCount: number; duration: number }[]
  }
) {
  mode.value = source
  cutoffAt.value = res.cutoffAt || ''

  const computedAt = res.snapshotAt || new Date().toISOString()
  const periodStart = fakeISODate(res.periodStart, '00:00')
  const periodEnd = fakeISODate(res.periodEnd, res.cutoffAt || '00:00')

  movies.value = (res.movies || []).map((item) => ({
    id: `${source}-${period.value}-media_movie-${item.rank}-${computedAt}`,
    period: period.value,
    category: 'media_movie',
    rank: item.rank,
    itemName: item.itemName,
    playCount: item.playCount,
    duration: item.duration,
    snapshotAt: computedAt,
    periodStart,
    periodEnd,
    createdAt: computedAt
  }))

  episodes.value = (res.episodes || []).map((item) => ({
    id: `${source}-${period.value}-media_episode-${item.rank}-${computedAt}`,
    period: period.value,
    category: 'media_episode',
    rank: item.rank,
    itemName: item.itemName,
    playCount: item.playCount,
    duration: item.duration,
    snapshotAt: computedAt,
    periodStart,
    periodEnd,
    createdAt: computedAt
  }))
}

async function runPreview() {
  if (!authStore.isAdmin) return

  loading.value = true
  try {
    const res = await previewRanking(period.value)
    applyComputedList('preview', res)

    ElMessage.success('预览生成完成（不入库、不推送）')
  } catch (err) {
    ElMessage.error('预览生成失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    loading.value = false
  }
}

async function runHistory() {
  if (!selectedDate.value) {
    ElMessage.warning('请选择日期')
    return
  }

  loading.value = true
  try {
    const res = await getRankingHistory(period.value, selectedDate.value)
    applyComputedList('history', res)
    ElMessage.success('历史数据已加载')
  } catch (err) {
    ElMessage.error('获取历史数据失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    loading.value = false
  }
}

async function handlePeriodChange() {
  // 切换日榜/周榜时，保持当前模式的语义
  if (!selectedDate.value) {
    selectedDate.value = toYMD(new Date())
  }

  if (mode.value === 'preview') {
    await runPreview()
    return
  }
  if (mode.value === 'history') {
    await runHistory()
    return
  }
  await fetchLatestAll()
}

onMounted(() => {
  selectedDate.value = toYMD(new Date())
  fetchLatestAll()
})
</script>

<template>
  <div class="space-y-6 animate-fade-in">
    <div class="flex items-start md:items-center justify-between gap-4 flex-col md:flex-row">
      <div class="flex items-center gap-3">
        <div class="p-2 rounded-xl bg-ember/10 text-ember">
          <el-icon :size="20"><Trophy /></el-icon>
        </div>
        <div>
          <h1 class="text-2xl font-bold text-gray-900">播放排行榜</h1>
          <div class="flex items-center gap-2 mt-1">
            <p v-if="rangeTextWithCutoff" class="text-sm text-gray-500 flex items-center gap-1">
              <el-icon :size="14" class="text-gray-400"><Calendar /></el-icon>
              <span>{{ rangeTextWithCutoff }}</span>
            </p>
            <span
              v-if="mode === 'preview'"
              class="text-[11px] px-2 py-0.5 rounded-full bg-amber-50 text-amber-700 border border-amber-100 font-semibold"
            >预览中</span>
            <span
              v-if="mode === 'history'"
              class="text-[11px] px-2 py-0.5 rounded-full bg-sky-50 text-sky-700 border border-sky-100 font-semibold"
            >历史</span>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <el-radio-group v-model="period" @change="handlePeriodChange">
          <el-radio-button label="daily">日榜</el-radio-button>
          <el-radio-button label="weekly">周榜</el-radio-button>
        </el-radio-group>

        <el-date-picker
          v-model="selectedDate"
          :type="period === 'daily' ? 'date' : 'week'"
          value-format="YYYY-MM-DD"
          placeholder="选择日期"
          style="width: 170px"
          :disabled="loading"
        />

        <el-button :disabled="loading" @click="runHistory">查看历史</el-button>

        <el-button
          v-if="mode === 'history'"
          :disabled="loading"
          @click="fetchLatestAll"
        >返回最新</el-button>

        <el-button
          v-if="authStore.isAdmin"
          :loading="loading && mode === 'preview'"
          type="primary"
          plain
          @click="runPreview"
        >预览生成</el-button>

        <el-button
          v-if="authStore.isAdmin && mode === 'preview'"
          :disabled="loading"
          @click="fetchLatestAll"
        >恢复最新</el-button>
      </div>
    </div>

    <el-skeleton v-if="loading" :rows="6" animated />

    <template v-else>
      <div v-if="movies.length === 0 && episodes.length === 0" class="bg-white border border-gray-100 rounded-2xl p-8">
        <el-empty description="暂无播放数据" />
        <p class="text-center text-sm text-gray-500 mt-4">{{ periodHint }}</p>
      </div>

      <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div class="bg-white border border-gray-100 rounded-2xl p-5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <el-icon class="text-ember" :size="18"><Film /></el-icon>
              <h2 class="text-lg font-semibold text-gray-900">电影 TOP 10</h2>
            </div>
            <span v-if="rangeText" class="text-xs text-gray-400 flex items-center gap-1">
              <el-icon :size="12" class="text-gray-300"><Calendar /></el-icon>
              <span>{{ rangeText }}</span>
            </span>
          </div>

          <div v-if="movies.length === 0" class="mt-6">
            <el-empty description="暂无电影排行" />
          </div>

          <div v-else class="mt-4 space-y-2">
            <div
              v-for="item in movies"
              :key="item.id"
              class="flex items-center gap-3 px-3 py-2 rounded-xl hover:bg-gray-50 transition-colors"
            >
              <div class="w-9 flex items-center justify-center">
                <span
                  v-if="item.rank <= 3"
                  class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-white"
                  :class="rankBadgeClass(item.rank)"
                >
                  {{ item.rank }}
                </span>
                <span v-else class="w-7 text-sm font-semibold text-gray-400 text-center tabular-nums">
                  {{ item.rank }}
                </span>
              </div>
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-gray-900 truncate">{{ item.itemName }}</div>
                <div class="text-xs text-gray-500 mt-0.5 flex items-center flex-wrap gap-x-2 gap-y-1">
                  <span class="inline-flex items-center">
                    <el-icon :size="12" class="mr-1 text-gray-400"><Timer /></el-icon>
                    <span>{{ formatDuration(item.duration) }}</span>
                  </span>
                  <span class="text-gray-300">·</span>
                  <span class="inline-flex items-center">
                    <el-icon :size="12" class="mr-1 text-gray-400"><VideoPlay /></el-icon>
                    <span>{{ item.playCount }} 次</span>
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="bg-white border border-gray-100 rounded-2xl p-5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <el-icon class="text-ember" :size="18"><VideoCamera /></el-icon>
              <h2 class="text-lg font-semibold text-gray-900">剧集 TOP 10</h2>
            </div>
            <span v-if="rangeText" class="text-xs text-gray-400 flex items-center gap-1">
              <el-icon :size="12" class="text-gray-300"><Calendar /></el-icon>
              <span>{{ rangeText }}</span>
            </span>
          </div>

          <div v-if="episodes.length === 0" class="mt-6">
            <el-empty description="暂无剧集排行" />
          </div>

          <div v-else class="mt-4 space-y-2">
            <div
              v-for="item in episodes"
              :key="item.id"
              class="flex items-center gap-3 px-3 py-2 rounded-xl hover:bg-gray-50 transition-colors"
            >
              <div class="w-9 flex items-center justify-center">
                <span
                  v-if="item.rank <= 3"
                  class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-white"
                  :class="rankBadgeClass(item.rank)"
                >
                  {{ item.rank }}
                </span>
                <span v-else class="w-7 text-sm font-semibold text-gray-400 text-center tabular-nums">
                  {{ item.rank }}
                </span>
              </div>
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-gray-900 truncate">{{ item.itemName }}</div>
                <div class="text-xs text-gray-500 mt-0.5 flex items-center flex-wrap gap-x-2 gap-y-1">
                  <span class="inline-flex items-center">
                    <el-icon :size="12" class="mr-1 text-gray-400"><Timer /></el-icon>
                    <span>{{ formatDuration(item.duration) }}</span>
                  </span>
                  <span class="text-gray-300">·</span>
                  <span class="inline-flex items-center">
                    <el-icon :size="12" class="mr-1 text-gray-400"><VideoPlay /></el-icon>
                    <span>{{ item.playCount }} 次</span>
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.4s ease-out;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
