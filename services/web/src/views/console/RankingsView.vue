<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Trophy, Film, VideoCamera, Calendar, Timer, VideoPlay } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { getLatestRanking, getRankingHistory } from '@/api/console'
import { previewRanking } from '@/api/admin'
import type { RankingItem, RankingPeriod, RankingResponse } from '@/types/api'

const period = ref<RankingPeriod>('daily')
const loading = ref(false)

const movies = ref<RankingItem[]>([])
const episodes = ref<RankingItem[]>([])
const mode = ref<'latest' | 'preview' | 'history'>('latest')
const cutoffAt = ref('')
const selectedDate = ref('')
const periodStart = ref('')
const periodEnd = ref('')

const authStore = useAuthStore()

const periodHint = computed(() => {
  return period.value === 'daily'
    ? '暂无播放数据，日榜将在每天 20:00 自动生成（阶段榜）'
    : '暂无播放数据，周榜将在每周日 20:30 自动生成（阶段榜）'
})

const rangeText = computed(() => {
  const start = periodStart.value || ''
  const end = periodEnd.value || ''
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

function clearRankingState() {
  movies.value = []
  episodes.value = []
  cutoffAt.value = ''
  periodStart.value = ''
  periodEnd.value = ''
}

function applyRanking(source: 'latest' | 'preview' | 'history', res: RankingResponse) {
  mode.value = source
  cutoffAt.value = res.cutoffAt || ''
  periodStart.value = res.periodStart || ''
  periodEnd.value = res.periodEnd || ''
  movies.value = res.movies || []
  episodes.value = res.episodes || []
}

function toYMD(d: Date): string {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

async function fetchLatestAll() {
  loading.value = true
  mode.value = 'latest'
  try {
    const res = await getLatestRanking(period.value)
    applyRanking('latest', res)
  } catch (err) {
    clearRankingState()
    ElMessage.error('获取排行榜失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    loading.value = false
  }
}

async function runPreview() {
  if (!authStore.isAdmin) return

  loading.value = true
  try {
    const res = await previewRanking(period.value)
    applyRanking('preview', res)

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
    applyRanking('history', res)
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

      <div class="flex w-full flex-wrap items-center gap-3 md:w-auto md:justify-end">
        <div class="inline-flex rounded-2xl bg-gray-100 p-1">
          <button
            type="button"
            class="rounded-xl px-4 py-2.5 text-sm font-semibold transition-colors cursor-pointer"
            :class="period === 'daily' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
            @click="period = 'daily'; handlePeriodChange()"
          >
            日榜
          </button>
          <button
            type="button"
            class="rounded-xl px-4 py-2.5 text-sm font-semibold transition-colors cursor-pointer"
            :class="period === 'weekly' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
            @click="period = 'weekly'; handlePeriodChange()"
          >
            周榜
          </button>
        </div>

        <div class="w-[172px]">
          <el-date-picker
            v-model="selectedDate"
            :type="period === 'daily' ? 'date' : 'week'"
            value-format="YYYY-MM-DD"
            placeholder="选择日期"
            class="w-full ranking-date"
            :disabled="loading"
          />
        </div>

        <button
          type="button"
          :disabled="loading"
          class="inline-flex h-[42px] items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
          @click="runHistory"
        >
          查看历史
        </button>

        <button
          v-if="mode === 'history'"
          type="button"
          :disabled="loading"
          class="inline-flex h-[42px] items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
          @click="fetchLatestAll"
        >
          返回最新
        </button>

        <button
          v-if="authStore.isAdmin"
          type="button"
          :disabled="loading"
          class="btn-ember inline-flex h-[42px] items-center justify-center rounded-xl px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
          @click="runPreview"
        >
          {{ loading && mode === 'preview' ? '预览生成中...' : '预览生成' }}
        </button>

        <button
          v-if="authStore.isAdmin && mode === 'preview'"
          type="button"
          :disabled="loading"
          class="inline-flex h-[42px] items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
          @click="fetchLatestAll"
        >
          恢复最新
        </button>
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
              :key="`${item.itemKey || item.itemName}-${item.rank}`"
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
              :key="`${item.itemKey || item.itemName}-${item.rank}`"
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
.ranking-date {
  --el-component-size: 42px;
  --el-date-editor-width: 172px;
  width: 172px !important;
  height: 42px;
}

:deep(.ranking-date.el-date-editor.el-input) {
  width: 172px !important;
  height: 42px !important;
  border-radius: 0.75rem !important;
}

:deep(.ranking-date.el-date-editor.el-input .el-input__wrapper) {
  height: 42px !important;
  min-height: 42px !important;
  border-radius: 0.75rem !important;
  background-color: #f9fafb !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  overflow: hidden;
}

:deep(.ranking-date.el-date-editor.el-input:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.ranking-date.el-date-editor.el-input.is-focus .el-input__wrapper) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.ranking-date .el-input__inner) {
  height: 100%;
  font-size: 0.875rem;
}

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
