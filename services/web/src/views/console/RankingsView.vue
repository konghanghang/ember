<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Trophy, Film, VideoCamera, Calendar, Timer, VideoPlay } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import { useAuthStore } from '@/store/auth'
import { getLatestRanking, getRankingHistory } from '@/api/console'
import { getRankingLibraryAllowlist, previewRanking, updateRankingLibraryAllowlist } from '@/api/admin'
import type { MediaLibraryOption, RankingItem, RankingPeriod, RankingResponse } from '@/types/api'

const authStore = useAuthStore()

const period = ref<RankingPeriod>('daily')
const loading = ref(false)

const movies = ref<RankingItem[]>([])
const episodes = ref<RankingItem[]>([])
const mode = ref<'latest' | 'preview' | 'history'>('latest')
const cutoffAt = ref('')
const selectedDate = ref('')
const periodStart = ref('')
const periodEnd = ref('')
const allowlistLoading = ref(authStore.isAdmin)
const allowlistSaving = ref(false)
const availableLibraries = ref<MediaLibraryOption[]>([])
const selectedLibraryIds = ref<string[]>([])
const invalidLibraryIds = ref<string[]>([])
const allowlistAppliesToAll = ref(true)

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

const periodTabs = computed(() => [
  { key: 'daily', label: '日榜' },
  { key: 'weekly', label: '周榜' }
])

const hasSelectedLibraries = computed(() => selectedLibraryIds.value.length > 0)
const hasInvalidOnlyAllowlist = computed(() => !allowlistAppliesToAll.value && !hasSelectedLibraries.value && invalidLibraryIds.value.length > 0)
const selectedLibrarySet = computed(() => new Set(selectedLibraryIds.value))
const selectedLibraryNames = computed(() => {
  if (!hasSelectedLibraries.value) {
    return []
  }
  return availableLibraries.value
    .filter(item => selectedLibrarySet.value.has(item.id))
    .map(item => item.name)
})

const allowlistSummary = computed(() => {
  if (hasInvalidOnlyAllowlist.value) {
    return '当前配置仅包含失效媒体库'
  }
  if (allowlistAppliesToAll.value && !hasSelectedLibraries.value) {
    return '当前按全部媒体库统计'
  }
  return `当前按 ${selectedLibraryIds.value.length} 个媒体库统计`
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

function applyAllowlistSettings(data?: {
  allowAll?: boolean
  libraryIds?: string[]
  libraries?: MediaLibraryOption[]
  invalidLibraryIds?: string[]
}) {
  allowlistAppliesToAll.value = data?.allowAll !== false
  availableLibraries.value = data?.libraries ?? []
  invalidLibraryIds.value = data?.invalidLibraryIds ?? []

  const validIds = new Set(availableLibraries.value.map(item => item.id))
  selectedLibraryIds.value = (data?.libraryIds ?? []).filter(id => validIds.has(id))
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

async function fetchRankingAllowlist() {
  if (!authStore.isAdmin) return

  allowlistLoading.value = true
  try {
    const res = await getRankingLibraryAllowlist()
    applyAllowlistSettings(res.data)
  } catch (err) {
    ElMessage.error('获取排行榜媒体库配置失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    allowlistLoading.value = false
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

async function saveRankingAllowlist() {
  if (!authStore.isAdmin) return

  allowlistSaving.value = true
  try {
    const res = await updateRankingLibraryAllowlist(selectedLibraryIds.value)
    applyAllowlistSettings(res.data)
    ElMessage.success(
      selectedLibraryIds.value.length === 0
        ? '已恢复为全部媒体库参与统计'
        : '排行榜统计媒体库已保存'
    )

    if (mode.value === 'preview') {
      await runPreview()
    }
  } catch (err) {
    ElMessage.error('保存排行榜媒体库配置失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    allowlistSaving.value = false
  }
}

async function resetRankingAllowlistToAll() {
  if (!authStore.isAdmin) return

  const previousSelected = [...selectedLibraryIds.value]
  const previousAllowAll = allowlistAppliesToAll.value
  const previousInvalid = [...invalidLibraryIds.value]
  selectedLibraryIds.value = []
  allowlistAppliesToAll.value = true
  invalidLibraryIds.value = []
  allowlistSaving.value = true
  try {
    const res = await updateRankingLibraryAllowlist([])
    applyAllowlistSettings(res.data)
    ElMessage.success('已恢复为全部媒体库参与统计')

    if (mode.value === 'preview') {
      await runPreview()
    }
  } catch (err) {
    selectedLibraryIds.value = previousSelected
    allowlistAppliesToAll.value = previousAllowAll
    invalidLibraryIds.value = previousInvalid
    ElMessage.error('保存排行榜媒体库配置失败')
    // eslint-disable-next-line no-console
    console.error(err)
  } finally {
    allowlistSaving.value = false
  }
}

onMounted(() => {
  selectedDate.value = toYMD(new Date())
  fetchLatestAll()
  fetchRankingAllowlist()
})
</script>

<template>
  <div class="space-y-6 animate-fade-in">
    <EmberPageHeaderCard
      title="播放排行榜"
      description="查看电影与剧集在当前时间窗口内的播放排行"
    >
      <template #actions>
        <div class="flex w-full flex-wrap items-center gap-3 md:w-auto md:justify-end">
          <EmberSegmentTabs
            v-model="period"
            :tabs="periodTabs"
            :full-width="false"
            aria-label="排行周期切换"
            @change="handlePeriodChange"
          />

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
      </template>

      <div v-if="rangeTextWithCutoff || mode !== 'latest'" class="mt-4 flex flex-wrap items-center gap-2">
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
    </EmberPageHeaderCard>

    <section
      v-if="authStore.isAdmin"
      class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm"
    >
      <div class="border-b border-gray-100 px-6 py-5">
        <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">参与统计的媒体库</h2>
            <p class="text-sm text-gray-500">控制日榜、周榜、预览生成与 Telegram 推送的统计范围。</p>
          </div>
          <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">
            {{ allowlistSummary }}
          </span>
        </div>
      </div>

      <div class="space-y-4 p-6">
        <div
          v-if="invalidLibraryIds.length > 0"
          class="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
        >
          当前配置包含 {{ invalidLibraryIds.length }} 个已失效媒体库，保存后会自动清理。
        </div>

        <el-skeleton v-if="allowlistLoading" :rows="3" animated />

        <EmberEmptyStateCard
          v-else-if="availableLibraries.length === 0"
          :icon="Trophy"
          tone="warning"
          title="当前没有可选媒体库"
          description="请先确认 Emby 已配置且媒体库列表可正常读取。"
        />

        <template v-else>
          <p class="text-sm text-gray-500">
            {{ hasInvalidOnlyAllowlist ? '当前配置已失效，保存后可恢复为全部媒体库统计或重新选择有效媒体库。' : '未选择任何媒体库时，默认统计全部媒体库。' }}
          </p>

          <el-checkbox-group
            v-model="selectedLibraryIds"
            class="grid gap-3 md:grid-cols-2 xl:grid-cols-3"
          >
            <div
              v-for="library in availableLibraries"
              :key="library.id"
              class="flex cursor-pointer items-start gap-3 rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 transition-colors hover:border-gray-300 hover:bg-white"
            >
              <el-checkbox :label="library.id" class="mt-0.5 !mr-0" />
              <div class="min-w-0">
                <p class="text-sm font-medium text-gray-900">{{ library.name }}</p>
                <p class="mt-1 text-xs text-gray-500">
                  {{ library.type || 'Unknown' }}<span v-if="library.itemCount !== undefined"> · {{ library.itemCount }} 项</span>
                </p>
              </div>
            </div>
          </el-checkbox-group>

          <div v-if="selectedLibraryNames.length > 0" class="flex flex-wrap gap-2">
            <span
              v-for="name in selectedLibraryNames"
              :key="name"
              class="inline-flex items-center rounded-full bg-ember/10 px-3 py-1 text-xs font-medium text-ember"
            >
              {{ name }}
            </span>
          </div>

          <div class="flex flex-wrap items-center justify-end gap-3">
            <button
              type="button"
              class="inline-flex h-[42px] items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="allowlistSaving || (allowlistAppliesToAll && invalidLibraryIds.length === 0 && !hasSelectedLibraries)"
              @click="resetRankingAllowlistToAll"
            >
              恢复全库统计
            </button>

            <button
              type="button"
              class="btn-ember inline-flex h-[42px] items-center justify-center rounded-xl px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="allowlistSaving"
              @click="saveRankingAllowlist"
            >
              {{ allowlistSaving ? '保存中...' : '保存媒体库范围' }}
            </button>
          </div>
        </template>
      </div>
    </section>

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
