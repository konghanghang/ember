<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Trophy, Film, VideoCamera, Calendar, Timer, VideoPlay } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
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
const allowlistDialogVisible = ref(false)
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

const isHistoryMode = computed(() => mode.value === 'history')
const isPreviewMode = computed(() => mode.value === 'preview')
const periodLabel = computed(() => (period.value === 'daily' ? '日榜' : '周榜'))
const modeLabel = computed(() => {
  if (isPreviewMode.value) return '预览结果'
  if (isHistoryMode.value) return '历史快照'
  return '最新榜单'
})

// 汇总当前榜单内的播放次数，避免在模板里重复遍历同一数组。
function sumPlayCount(items: RankingItem[]): number {
  return items.reduce((total, item) => total + item.playCount, 0)
}

const movieTotalPlays = computed(() => sumPlayCount(movies.value))
const episodeTotalPlays = computed(() => sumPlayCount(episodes.value))
const totalPlays = computed(() => movieTotalPlays.value + episodeTotalPlays.value)
const rankedItemCount = computed(() => movies.value.length + episodes.value.length)
const movieLeader = computed(() => movies.value[0] ?? null)
const episodeLeader = computed(() => episodes.value[0] ?? null)
const movieOtherItems = computed(() => movies.value.slice(1))
const episodeOtherItems = computed(() => episodes.value.slice(1))

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
    allowlistDialogVisible.value = false

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
      description="按日榜或周榜查看电影与剧集播放热度"
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
              :placeholder="period === 'daily' ? '选择日期' : '选择周'"
              class="w-full !w-full input-ember form-date"
              :disabled="loading"
            />
          </div>

          <button
            type="button"
            :disabled="loading"
            class="inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors duration-200 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            @click="runHistory"
          >
            查看历史
          </button>

          <button
            v-if="isHistoryMode"
            type="button"
            :disabled="loading"
            class="inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors duration-200 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            @click="fetchLatestAll"
          >
            返回最新
          </button>

          <button
            v-if="authStore.isAdmin"
            type="button"
            :disabled="allowlistLoading"
            data-test="open-allowlist-dialog"
            class="inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors duration-200 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            @click="allowlistDialogVisible = true"
          >
            {{ allowlistLoading ? '读取中...' : '媒体库范围' }}
          </button>

          <button
            v-if="authStore.isAdmin"
            type="button"
            :disabled="loading"
            class="btn-ember inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
            @click="runPreview"
          >
            {{ loading && isPreviewMode ? '预览生成中...' : '预览生成' }}
          </button>

          <button
            v-if="authStore.isAdmin && isPreviewMode"
            type="button"
            :disabled="loading"
            class="inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors duration-200 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            @click="fetchLatestAll"
          >
            恢复最新
          </button>
        </div>
      </template>

      <div class="mt-4 flex flex-wrap items-center gap-2">
        <span
          class="inline-flex items-center rounded-full border px-3 py-1 text-xs font-semibold"
          :class="isPreviewMode
            ? 'border-amber-200 bg-amber-50 text-amber-800'
            : isHistoryMode
              ? 'border-sky-200 bg-sky-50 text-sky-800'
              : 'border-emerald-200 bg-emerald-50 text-emerald-800'"
        >
          {{ periodLabel }} · {{ modeLabel }}
        </span>
        <span
          class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700"
        >
          统计窗口：{{ rangeTextWithCutoff || `${periodLabel}等待生成` }}
        </span>
        <span
          class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700"
        >
          总播放：{{ totalPlays }}
        </span>
        <span
          class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700"
        >
          上榜条目：{{ rankedItemCount }}
        </span>
        <span
          v-if="authStore.isAdmin"
          class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700"
        >
          媒体库：{{ allowlistSummary }}
        </span>
        <span
          v-if="invalidLibraryIds.length > 0"
          class="inline-flex items-center rounded-full bg-amber-50 px-3 py-1 text-xs font-medium text-amber-800"
        >
          有 {{ invalidLibraryIds.length }} 个失效媒体库
        </span>
        <span
          v-if="isPreviewMode"
          class="inline-flex items-center rounded-full bg-white px-3 py-1 text-xs font-medium text-gray-500 ring-1 ring-inset ring-gray-200"
        >
          预览不会入库或推送
        </span>
      </div>
    </EmberPageHeaderCard>

    <EmberFormDialog
      v-if="authStore.isAdmin"
      v-model="allowlistDialogVisible"
      title="配置统计媒体库"
      width="min(920px, calc(100vw - 24px))"
    >
      <div class="space-y-4 px-6 pb-2 pt-2">
        <div class="grid gap-3 md:grid-cols-3">
          <div class="rounded-2xl border border-gray-200 bg-stone-50 px-4 py-4">
            <p class="text-xs font-semibold tracking-wide text-gray-400">当前设置</p>
            <p class="mt-2 text-base font-semibold text-gray-900">{{ allowlistSummary }}</p>
            <p class="mt-1 text-xs leading-5 text-gray-500">
              {{ hasInvalidOnlyAllowlist ? '当前配置已失效，保存后会清理失效媒体库。' : '不选任何媒体库时，默认统计全部媒体库。' }}
            </p>
          </div>

          <div class="rounded-2xl border border-gray-200 bg-stone-50 px-4 py-4">
            <p class="text-xs font-semibold tracking-wide text-gray-400">已选媒体库</p>
            <p class="mt-2 text-base font-semibold text-gray-900">
              {{ hasSelectedLibraries ? `${selectedLibraryIds.length} 个` : '全部媒体库' }}
            </p>
            <p class="mt-1 text-xs leading-5 text-gray-500">
              {{ hasSelectedLibraries ? '当前只统计勾选项。' : '未限制媒体库范围。' }}
            </p>
          </div>

          <div class="rounded-2xl border border-gray-200 bg-stone-50 px-4 py-4">
            <p class="text-xs font-semibold tracking-wide text-gray-400">影响范围</p>
            <p class="mt-2 text-base font-semibold text-gray-900">日榜、周榜、预览、推送</p>
            <p class="mt-1 text-xs leading-5 text-gray-500">
              配置会同步影响排行榜生成和 Telegram 推送范围。
            </p>
          </div>
        </div>

        <div
          v-if="invalidLibraryIds.length > 0"
          class="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
        >
          当前配置包含 {{ invalidLibraryIds.length }} 个已失效媒体库，保存后会自动清理。
        </div>

        <div class="rounded-3xl border border-gray-200 bg-white">
          <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-5 py-4">
            <div>
              <h2 class="text-base font-semibold text-gray-900">参与统计的媒体库</h2>
              <p class="mt-1 text-sm text-gray-500">勾选需要参与排行榜统计的媒体库。</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <span class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700">
                可选 {{ availableLibraries.length }} 个
              </span>
              <span
                v-if="hasSelectedLibraries"
                class="inline-flex items-center rounded-full bg-ember/10 px-3 py-1 text-xs font-medium text-ember"
              >
                已选 {{ selectedLibraryIds.length }} 个
              </span>
            </div>
          </div>

          <el-skeleton v-if="allowlistLoading" :rows="3" animated />

          <EmberEmptyStateCard
            v-else-if="availableLibraries.length === 0"
            :icon="Trophy"
            tone="warning"
            title="当前没有可选媒体库"
            description="请先确认 Emby 已配置且媒体库列表可正常读取。"
          />

          <template v-else-if="availableLibraries.length > 0">
            <div class="max-h-[420px] overflow-y-auto px-5 py-4">
              <el-checkbox-group
                v-model="selectedLibraryIds"
                class="grid gap-3 md:grid-cols-2"
              >
                <label
                  v-for="library in availableLibraries"
                  :key="library.id"
                  class="flex cursor-pointer items-start gap-3 rounded-2xl border px-4 py-4 transition-colors duration-200"
                  :class="selectedLibrarySet.has(library.id)
                    ? 'border-ember/30 bg-ember/5'
                    : 'border-gray-200 bg-white hover:border-gray-300 hover:bg-stone-50/60'"
                >
                  <el-checkbox
                    :label="library.id"
                    :aria-label="`选择媒体库 ${library.name}`"
                    class="mt-1 !mr-0 shrink-0"
                  >
                    <span class="sr-only">{{ library.name }}</span>
                  </el-checkbox>
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-start justify-between gap-2">
                      <div class="min-w-0">
                        <p class="truncate text-sm font-semibold text-gray-900">{{ library.name }}</p>
                        <p class="mt-1 text-xs text-gray-500">
                          {{
                            library.type === 'movies'
                              ? '电影库'
                              : library.type === 'tvshows'
                              ? '剧集库'
                                : library.type || 'Unknown'
                          }}<span v-if="library.itemCount !== undefined"> · {{ library.itemCount }} 项</span>
                          <span class="text-gray-300"> · </span>
                          <span class="font-mono text-[11px] text-gray-400">{{ library.id }}</span>
                        </p>
                      </div>
                      <span
                        class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium"
                        :class="selectedLibrarySet.has(library.id)
                          ? 'bg-ember/10 text-ember'
                          : 'bg-stone-100 text-gray-500'"
                      >
                        {{ selectedLibrarySet.has(library.id) ? '已选择' : '未选择' }}
                      </span>
                    </div>
                  </div>
                </label>
              </el-checkbox-group>
            </div>

            <div
              v-if="selectedLibraryNames.length > 0"
              class="border-t border-gray-100 px-5 py-4"
            >
              <p class="text-xs font-semibold tracking-wide text-gray-400">当前已选</p>
              <div class="mt-3 flex flex-wrap gap-2">
                <span
                  v-for="name in selectedLibraryNames"
                  :key="name"
                  class="inline-flex items-center rounded-full bg-ember/10 px-3 py-1 text-xs font-medium text-ember"
                >
                  {{ name }}
                </span>
              </div>
            </div>
          </template>
        </div>
      </div>
      <template #footer>
        <div class="flex flex-wrap justify-end gap-3 px-6 pb-6 pt-2">
          <button
            type="button"
            class="inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors duration-200 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            @click="allowlistDialogVisible = false"
          >
            取消
          </button>
          <button
            type="button"
            class="inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 transition-colors duration-200 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="allowlistSaving || (allowlistAppliesToAll && invalidLibraryIds.length === 0 && !hasSelectedLibraries)"
            @click="resetRankingAllowlistToAll"
          >
            恢复全库统计
          </button>
          <button
            type="button"
            class="btn-ember inline-flex h-[42px] cursor-pointer items-center justify-center rounded-xl px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="allowlistSaving"
            @click="saveRankingAllowlist"
          >
            {{ allowlistSaving ? '保存中...' : '保存媒体库范围' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <el-skeleton v-if="loading" :rows="6" animated />

    <template v-else>
      <EmberEmptyStateCard
        v-if="movies.length === 0 && episodes.length === 0"
        :icon="Trophy"
        title="暂无播放数据"
        :description="periodHint"
      />

      <div v-else class="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <section class="overflow-hidden rounded-[28px] border border-gray-100 bg-white shadow-sm">
          <div class="border-b border-gray-100 px-6 py-5">
            <div class="flex flex-wrap items-end justify-between gap-3">
              <div class="min-w-0">
                <p class="text-[11px] font-semibold tracking-[0.22em] text-gray-400">电影榜单</p>
                <div class="mt-3 flex items-center gap-3">
                  <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-ember/10 text-ember">
                    <el-icon :size="18"><Film /></el-icon>
                  </div>
                  <div>
                    <h2 class="text-2xl font-bold tracking-[-0.04em] text-gray-950">电影 TOP 10</h2>
                    <p class="text-sm text-gray-500">按播放次数排序</p>
                  </div>
                </div>
              </div>
              <div class="text-right">
                <p class="text-xs font-semibold tracking-wide text-gray-400">榜内播放</p>
                <p class="text-2xl font-bold tracking-[-0.04em] text-gray-950">{{ movieTotalPlays }}</p>
              </div>
            </div>
          </div>

          <div class="p-6">
            <EmberEmptyStateCard
              v-if="movies.length === 0"
              :icon="Film"
              compact
              title="暂无电影排行"
              description="当前时间窗口内还没有电影上榜。"
            />

            <template v-else>
              <article class="rounded-3xl border border-gray-200 bg-stone-50 p-5">
                <div class="grid gap-4 md:grid-cols-[72px_minmax(0,1fr)_auto] md:items-end">
                  <div class="text-[3.25rem] font-black leading-none tracking-[-0.08em] text-ember">01</div>
                  <div class="min-w-0">
                    <p class="text-xs font-semibold uppercase tracking-[0.28em] text-gray-400">榜首</p>
                    <h3 class="mt-2 break-words text-xl font-semibold leading-7 text-gray-950">
                      {{ movieLeader?.itemName }}
                    </h3>
                  </div>
                  <div class="flex flex-wrap gap-2 md:justify-end">
                    <span class="inline-flex items-center rounded-full bg-white px-3 py-1 text-xs font-medium text-gray-600 ring-1 ring-inset ring-gray-200">
                      <el-icon :size="12" class="mr-1 text-gray-400"><VideoPlay /></el-icon>
                      {{ movieLeader?.playCount }} 次播放
                    </span>
                    <span class="inline-flex items-center rounded-full bg-white px-3 py-1 text-xs font-medium text-gray-600 ring-1 ring-inset ring-gray-200">
                      <el-icon :size="12" class="mr-1 text-gray-400"><Timer /></el-icon>
                      {{ formatDuration(movieLeader?.duration ?? 0) }}
                    </span>
                  </div>
                </div>
              </article>

              <div v-if="movieOtherItems.length > 0" class="mt-4 border-t border-gray-100 pt-2">
                <article
                  v-for="item in movieOtherItems"
                  :key="`${item.itemKey || item.itemName}-${item.rank}`"
                  class="grid gap-3 border-b border-gray-100 px-1 py-4 last:border-b-0 md:grid-cols-[44px_minmax(0,1fr)_auto]"
                >
                  <div class="text-lg font-bold tracking-[-0.04em] text-gray-400">
                    {{ String(item.rank).padStart(2, '0') }}
                  </div>
                  <div class="min-w-0">
                    <h3 class="truncate text-sm font-semibold text-gray-900">{{ item.itemName }}</h3>
                    <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
                      <span class="inline-flex items-center">
                        <el-icon :size="12" class="mr-1 text-gray-400"><Timer /></el-icon>
                        <span>{{ formatDuration(item.duration) }}</span>
                      </span>
                    </div>
                  </div>
                  <div class="flex items-center justify-start md:justify-end">
                    <span class="inline-flex items-center rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-gray-700">
                      <el-icon :size="12" class="mr-1 text-gray-400"><VideoPlay /></el-icon>
                      {{ item.playCount }} 次
                    </span>
                  </div>
                </article>
              </div>
            </template>
          </div>
        </section>

        <section class="overflow-hidden rounded-[28px] border border-gray-100 bg-white shadow-sm">
          <div class="border-b border-gray-100 px-6 py-5">
            <div class="flex flex-wrap items-end justify-between gap-3">
              <div class="min-w-0">
                <p class="text-[11px] font-semibold tracking-[0.22em] text-gray-400">剧集榜单</p>
                <div class="mt-3 flex items-center gap-3">
                  <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-gray-100 text-gray-700">
                    <el-icon :size="18"><VideoCamera /></el-icon>
                  </div>
                  <div>
                    <h2 class="text-2xl font-bold tracking-[-0.04em] text-gray-950">剧集 TOP 10</h2>
                    <p class="text-sm text-gray-500">按播放次数排序</p>
                  </div>
                </div>
              </div>
              <div class="text-right">
                <p class="text-xs font-semibold tracking-wide text-gray-400">榜内播放</p>
                <p class="text-2xl font-bold tracking-[-0.04em] text-gray-950">{{ episodeTotalPlays }}</p>
              </div>
            </div>
          </div>

          <div class="p-6">
            <EmberEmptyStateCard
              v-if="episodes.length === 0"
              :icon="VideoCamera"
              compact
              title="暂无剧集排行"
              description="当前时间窗口内还没有剧集上榜。"
            />

            <template v-else>
              <article class="rounded-3xl border border-gray-200 bg-stone-50 p-5">
                <div class="grid gap-4 md:grid-cols-[72px_minmax(0,1fr)_auto] md:items-end">
                  <div class="text-[3.25rem] font-black leading-none tracking-[-0.08em] text-gray-900">01</div>
                  <div class="min-w-0">
                    <p class="text-xs font-semibold uppercase tracking-[0.28em] text-gray-400">榜首</p>
                    <h3 class="mt-2 break-words text-xl font-semibold leading-7 text-gray-950">
                      {{ episodeLeader?.itemName }}
                    </h3>
                  </div>
                  <div class="flex flex-wrap gap-2 md:justify-end">
                    <span class="inline-flex items-center rounded-full bg-white px-3 py-1 text-xs font-medium text-gray-600 ring-1 ring-inset ring-gray-200">
                      <el-icon :size="12" class="mr-1 text-gray-400"><VideoPlay /></el-icon>
                      {{ episodeLeader?.playCount }} 次播放
                    </span>
                    <span class="inline-flex items-center rounded-full bg-white px-3 py-1 text-xs font-medium text-gray-600 ring-1 ring-inset ring-gray-200">
                      <el-icon :size="12" class="mr-1 text-gray-400"><Timer /></el-icon>
                      {{ formatDuration(episodeLeader?.duration ?? 0) }}
                    </span>
                  </div>
                </div>
              </article>

              <div v-if="episodeOtherItems.length > 0" class="mt-4 border-t border-gray-100 pt-2">
                <article
                  v-for="item in episodeOtherItems"
                  :key="`${item.itemKey || item.itemName}-${item.rank}`"
                  class="grid gap-3 border-b border-gray-100 px-1 py-4 last:border-b-0 md:grid-cols-[44px_minmax(0,1fr)_auto]"
                >
                  <div class="text-lg font-bold tracking-[-0.04em] text-gray-400">
                    {{ String(item.rank).padStart(2, '0') }}
                  </div>
                  <div class="min-w-0">
                    <h3 class="truncate text-sm font-semibold text-gray-900">{{ item.itemName }}</h3>
                    <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
                      <span class="inline-flex items-center">
                        <el-icon :size="12" class="mr-1 text-gray-400"><Timer /></el-icon>
                        <span>{{ formatDuration(item.duration) }}</span>
                      </span>
                    </div>
                  </div>
                  <div class="flex items-center justify-start md:justify-end">
                    <span class="inline-flex items-center rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-gray-700">
                        <el-icon :size="12" class="mr-1 text-gray-400"><VideoPlay /></el-icon>
                      {{ item.playCount }} 次
                    </span>
                  </div>
                </article>
              </div>
            </template>
          </div>
        </section>
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
