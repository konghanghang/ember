<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Calendar,
  CircleCheckFilled,
  Clock,
  Refresh,
  VideoPlay,
  WarningFilled
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import {
  getGlobalTVCalendar,
  getTVCalendarSubscriptions,
  subscribeTVCalendar,
  unsubscribeTVCalendar
} from '@/api/console'
import { syncTVCalendar } from '@/api/admin'
import type {
  TVCalendarStatus,
  TVCalendarSubscription,
  TVCalendarWeeklyData,
  TVCalendarWeeklyItem
} from '@/types/api'

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)

const loading = ref(false)
const refreshing = ref(false)
const calendarError = ref('')

const subscriptions = ref<TVCalendarSubscription[]>([])
const calendarData = ref<TVCalendarWeeklyData>({ dateRange: '', days: [] })

const filters = reactive({
  weekDate: formatDateLocal(new Date()),
  status: '' as TVCalendarStatus | ''
})

const statusOptions: Array<{ label: string; value: TVCalendarStatus | '' }> = [
  { label: '全部状态', value: '' },
  { label: '已入库', value: 'ready' },
  { label: '缺失', value: 'missing' },
  { label: '今日播出', value: 'today' },
  { label: '待播', value: 'upcoming' }
]

const subscriptionSet = computed(() => new Set(subscriptions.value.map((item) => item.tmdbId)))
const dayColumns = computed(() => calendarData.value.days || [])
const totalItems = computed(() => dayColumns.value.reduce((sum, day) => sum + day.items.length, 0))
const activeDayCount = computed(() => dayColumns.value.filter((day) => day.items.length > 0).length)
const readyCount = computed(() => countItemsByStatus('ready'))
const todayCount = computed(() => countItemsByStatus('today'))
const missingCount = computed(() => countItemsByStatus('missing'))
const hasCalendarItems = computed(() => totalItems.value > 0)
const emptyStateTitle = computed(() => '当前周历还没有可展示条目')
const emptyStateDescription = computed(() => '检查 Emby 连载剧识别、TMDB 配置，或者让管理员手动同步一次。')
const summaryCards = computed(() => [
  {
    title: '本周条目',
    value: totalItems.value,
    detail: `${activeDayCount.value} 个活跃日期`,
    icon: VideoPlay,
    tone: 'ink'
  },
  {
    title: '已入库',
    value: readyCount.value,
    detail: '可以直接观看',
    icon: CircleCheckFilled,
    tone: 'ready'
  },
  {
    title: '今日播出',
    value: todayCount.value,
    detail: '当天重点',
    icon: Clock,
    tone: 'today'
  },
  {
    title: '缺失集数',
    value: missingCount.value,
    detail: '已播但还未入库',
    icon: WarningFilled,
    tone: 'warning'
  }
])

function countItemsByStatus(status: TVCalendarStatus): number {
  return dayColumns.value.reduce(
    (sum, day) => sum + day.items.filter((item) => item.status === status).length,
    0
  )
}

function formatDateLocal(date: Date): string {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

function startOfWeekLocal(date: Date): Date {
  const result = new Date(date)
  const weekday = result.getDay() === 0 ? 7 : result.getDay()
  result.setHours(0, 0, 0, 0)
  result.setDate(result.getDate() - weekday + 1)
  return result
}

function extractErrorMessage(error: unknown, fallback: string): string {
  const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
  if (typeof message === 'string' && message.trim()) {
    return message.trim()
  }
  return fallback
}

function statusText(status: TVCalendarStatus): string {
  switch (status) {
    case 'ready':
      return '已入库'
    case 'today':
      return '今日播出'
    case 'missing':
      return '缺失'
    case 'upcoming':
      return '待播'
    default:
      return status
  }
}

function getPosterFallback(name: string): string {
  const value = name.trim()
  if (!value) {
    return 'TV'
  }
  return value.slice(0, 1).toUpperCase()
}

function isFollowing(tmdbId: string): boolean {
  return subscriptionSet.value.has(tmdbId)
}

function summaryCardClass(tone: string): string {
  switch (tone) {
    case 'ready':
      return 'tv-summary-card tv-summary-card-ready'
    case 'today':
      return 'tv-summary-card tv-summary-card-today'
    case 'warning':
      return 'tv-summary-card tv-summary-card-warning'
    default:
      return 'tv-summary-card tv-summary-card-ink'
  }
}

function statusChipClass(value: TVCalendarStatus | ''): string {
  return value === filters.status
    ? 'tv-status-chip tv-status-chip-active'
    : 'tv-status-chip'
}

function statusBadgeClass(status: TVCalendarStatus): string {
  switch (status) {
    case 'ready':
      return 'tv-status-badge tv-status-badge-ready'
    case 'today':
      return 'tv-status-badge tv-status-badge-today'
    case 'missing':
      return 'tv-status-badge tv-status-badge-missing'
    default:
      return 'tv-status-badge tv-status-badge-upcoming'
  }
}

function dayDateNumber(date: string): string {
  const [year, month, day] = date.split('-')
  if (!year || !month || !day) {
    return date
  }
  return day
}

function dayDateMonth(date: string): string {
  const [year, month] = date.split('-')
  if (!year || !month) {
    return ''
  }
  return `${Number(month)}月`
}

async function fetchSubscriptions(): Promise<void> {
  try {
    const res = await getTVCalendarSubscriptions()
    subscriptions.value = res.data || []
  } catch (error) {
    subscriptions.value = []
    ElMessage.error(extractErrorMessage(error, '读取关注列表失败'))
  }
}

async function fetchCalendar(): Promise<void> {
  loading.value = true
  calendarError.value = ''
  try {
    const params = {
      weekDate: filters.weekDate,
      status: filters.status || undefined
    }
    const res = await getGlobalTVCalendar(params)
    calendarData.value = res.data || { dateRange: '', days: [] }
  } catch (error) {
    calendarData.value = { dateRange: '', days: [] }
    calendarError.value = extractErrorMessage(error, '读取追剧日历失败')
  } finally {
    loading.value = false
  }
}

async function loadAll(): Promise<void> {
  await Promise.allSettled([fetchSubscriptions(), fetchCalendar()])
}

async function handleFollow(item: TVCalendarWeeklyItem): Promise<void> {
  if (isFollowing(item.tmdbId)) {
    await handleUnfollow(item.tmdbId, item.showName)
    return
  }

  await subscribeTVCalendar({
    tmdbId: item.tmdbId,
    showName: item.showName,
    posterUrl: item.posterUrl || undefined
  })
  ElMessage.success(`已关注《${item.showName}》`)
  await Promise.allSettled([fetchSubscriptions(), fetchCalendar()])
}

async function handleUnfollow(tmdbId: string, showName: string): Promise<void> {
  try {
    await ElMessageBox.confirm(`确认取消关注《${showName}》吗？`, '取消关注', {
      type: 'warning',
      confirmButtonText: '确认',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }

  await unsubscribeTVCalendar(tmdbId)
  ElMessage.success(`已取消关注《${showName}》`)
  await Promise.allSettled([fetchSubscriptions(), fetchCalendar()])
}

async function handleSync(): Promise<void> {
  refreshing.value = true
  try {
    const res = await syncTVCalendar({ force: false, weekOffsets: [-1, 0, 1] })
    ElMessage.success(`同步完成，处理 ${res.count} 条`)
    await fetchCalendar()
  } catch (error) {
    ElMessage.error(extractErrorMessage(error, '手动同步失败'))
  } finally {
    refreshing.value = false
  }
}

async function changeStatus(status: TVCalendarStatus | ''): Promise<void> {
  filters.status = status
  await fetchCalendar()
}

async function changeWeekDate(): Promise<void> {
  await fetchCalendar()
}

onMounted(() => {
  void loadAll()
})
</script>

<template>
  <div class="tv-calendar-shell">
    <section class="tv-panel overflow-hidden rounded-[32px]">
      <div class="tv-stage p-6 sm:p-8">
        <div class="space-y-6">
          <div class="max-w-3xl">
            <p class="tv-kicker">Ember Weekly Watchboard</p>
            <h1 class="tv-display mt-4 text-4xl leading-none text-slate-950 sm:text-5xl">追剧日历</h1>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <el-button
              v-if="isAdmin"
              class="!rounded-full !border-slate-300 !bg-white/80 !px-5 !text-slate-700 hover:!border-slate-950 hover:!text-slate-950"
              :icon="Refresh"
              :loading="refreshing"
              @click="handleSync"
            >
              手动同步
            </el-button>
          </div>

          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <article
              v-for="card in summaryCards"
              :key="card.title"
              :class="summaryCardClass(card.tone)"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-xs uppercase tracking-[0.24em] text-slate-400">{{ card.title }}</p>
                  <p class="mt-4 text-4xl font-semibold leading-none text-slate-950">{{ card.value }}</p>
                  <p class="mt-3 text-sm text-slate-500">{{ card.detail }}</p>
                </div>
                <div class="tv-summary-icon">
                  <el-icon :size="18">
                    <component :is="card.icon" />
                  </el-icon>
                </div>
              </div>
            </article>
          </div>

          <el-alert
            v-if="calendarError"
            class="tv-floating-alert"
            type="warning"
            :closable="false"
            show-icon
            title="追剧日历暂时不可用"
            :description="calendarError"
          />
        </div>
      </div>

      <div class="tv-board p-4 sm:p-6">
        <div class="flex flex-col gap-4 border-b border-slate-200/80 pb-5 xl:flex-row xl:items-end xl:justify-between">
          <div class="tv-toolbar">
            <div class="tv-picker-inline">
              <span class="tv-toolbar-label">日期</span>
              <div class="tv-picker-copy">
                <div class="tv-picker-field group">
                  <div class="tv-picker-icon">
                    <el-icon><Calendar /></el-icon>
                  </div>
                  <el-date-picker
                    v-model="filters.weekDate"
                    class="tv-week-picker filter-date"
                    type="date"
                    value-format="YYYY-MM-DD"
                    format="YYYY-MM-DD"
                    placeholder="选择任意日期"
                    :clearable="false"
                    @change="changeWeekDate"
                  />
                </div>
                <p class="tv-picker-hint">选择任意日期后，会自动定位到该日期所在周。</p>
              </div>
            </div>
            <div class="tv-status-group">
              <button
                v-for="option in statusOptions"
                :key="option.value"
                type="button"
                :class="statusChipClass(option.value)"
                :aria-pressed="filters.status === option.value"
                @click="changeStatus(option.value)"
              >
                {{ option.label }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="loading" class="mt-6 space-y-4">
          <div v-for="index in 7" :key="index" class="tv-day-row animate-pulse">
            <div class="tv-day-row-head">
              <div class="h-4 w-12 rounded-full bg-slate-200/70"></div>
              <div class="mt-3 h-10 w-16 rounded-2xl bg-slate-200/70"></div>
              <div class="mt-3 h-4 w-20 rounded-full bg-slate-200/60"></div>
            </div>
            <div class="tv-day-row-track">
              <div class="tv-day-row-strip">
                <div v-for="inner in 7" :key="inner" class="tv-mini-card rounded-[18px] bg-white/90">
                  <div class="h-24 rounded-[14px] bg-slate-200/70"></div>
                  <div class="mt-3 h-4 w-4/5 rounded-full bg-slate-200/70"></div>
                  <div class="mt-2 h-3 w-3/5 rounded-full bg-slate-200/60"></div>
                  <div class="mt-4 h-7 w-full rounded-full bg-slate-200/70"></div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-else-if="!hasCalendarItems"
          class="mt-6 rounded-[28px] border border-dashed border-slate-300 bg-[linear-gradient(180deg,#fffaf2_0%,#ffffff_100%)] px-6 py-16 text-center"
        >
          <p class="text-lg font-semibold text-slate-900">{{ emptyStateTitle }}</p>
          <p class="mx-auto mt-3 max-w-xl text-sm leading-7 text-slate-500">{{ emptyStateDescription }}</p>
          <div class="mt-6 flex justify-center gap-3">
            <el-button
              v-if="isAdmin"
              class="!rounded-full !border-slate-300 !bg-white/80 !px-6 !text-slate-700"
              :icon="Refresh"
              @click="handleSync"
            >
              立即同步
            </el-button>
          </div>
        </div>

        <div v-else class="mt-6 space-y-4">
          <article
            v-for="day in dayColumns"
            :key="day.date"
            class="tv-day-row"
            :class="{
              'tv-day-row-today': day.isToday,
              'tv-day-row-active': day.items.length > 0
            }"
          >
            <div class="tv-day-row-head">
              <div>
                <p class="text-xs font-medium uppercase tracking-[0.22em] text-slate-400">{{ dayDateMonth(day.date) }}</p>
                <div class="mt-2 flex items-end gap-1.5">
                  <span class="text-[2rem] font-semibold leading-none text-slate-950">{{ dayDateNumber(day.date) }}</span>
                  <span class="pb-1 text-sm text-slate-500">{{ day.weekdayCn }}</span>
                </div>
              </div>
              <span class="mt-3 inline-flex rounded-full border border-slate-200 bg-white/85 px-2.5 py-1 text-[11px] font-medium text-slate-600">
                {{ day.isToday ? '今天' : `${day.items.length} 集` }}
              </span>
            </div>

            <div v-if="day.items.length === 0" class="tv-day-row-empty">
              <p class="max-w-[220px] text-sm leading-6 text-slate-400">当天没有命中当前视图和状态筛选</p>
            </div>

            <div v-else class="tv-day-row-track">
              <div class="tv-day-row-strip">
                <div
                  v-for="item in day.items"
                  :key="`${day.date}-${item.tmdbId}-${item.season}-${item.episode}`"
                  class="tv-mini-card"
                >
                  <div class="tv-mini-poster-shell">
                    <div class="tv-mini-poster">
                      <img
                        v-if="item.posterUrl"
                        :src="item.posterUrl"
                        :alt="item.showName"
                        class="h-full w-full object-cover"
                        loading="lazy"
                      />
                      <div v-else class="flex h-full w-full items-center justify-center text-lg font-semibold text-slate-800">
                        {{ getPosterFallback(item.showName) }}
                      </div>
                    </div>
                    <div class="tv-mini-overlay">
                      <span :class="statusBadgeClass(item.status)">{{ statusText(item.status) }}</span>
                      <span class="tv-mini-code">
                        S{{ String(item.season).padStart(2, '0') }} · E{{ item.episode }}
                      </span>
                    </div>
                  </div>

                  <div class="mt-3">
                    <p class="truncate text-sm font-semibold text-slate-950">{{ item.showName }}</p>
                    <p
                      v-if="item.episodeName"
                      class="mt-1 line-clamp-1 text-[11px] leading-5 text-slate-500"
                    >
                      {{ item.episodeName }}
                    </p>
                    <div class="mt-3 flex items-center justify-between gap-2">
                      <span class="truncate text-[10px] font-medium uppercase tracking-[0.16em] text-slate-400">{{ item.airDate }}</span>
                    <button
                      type="button"
                      class="tv-card-action"
                      :class="{ 'tv-card-action-active': isFollowing(item.tmdbId) }"
                      @click="handleFollow(item)"
                    >
                      {{ isFollowing(item.tmdbId) ? '已收' : '关注' }}
                    </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </article>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.tv-calendar-shell {
  --tv-paper: #ffffff;
  --tv-ink: #111827;
  --tv-muted: #6b7280;
  --tv-line: rgba(148, 163, 184, 0.18);
  --tv-shadow: 0 14px 34px rgba(15, 23, 42, 0.05);
  --tv-shadow-soft: 0 8px 18px rgba(15, 23, 42, 0.035);
}

.tv-panel {
  border: 1px solid rgba(226, 232, 240, 0.88);
  background: #ffffff;
  box-shadow: var(--tv-shadow);
}

.tv-stage {
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(circle at top left, rgba(251, 191, 36, 0.08), transparent 26%),
    linear-gradient(180deg, #fffdf8 0%, #ffffff 68%);
}

.tv-stage::after {
  content: '';
  position: absolute;
  inset: auto -80px -110px auto;
  width: 280px;
  height: 280px;
  border-radius: 999px;
  background: radial-gradient(circle, rgba(15, 23, 42, 0.08), transparent 68%);
  pointer-events: none;
}

.tv-board {
  border-top: 1px solid rgba(226, 232, 240, 0.88);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(249, 250, 251, 0.8));
}

.tv-display {
  font-family: inherit;
  font-weight: 700;
  letter-spacing: -0.03em;
}

.tv-kicker {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.28em;
  text-transform: uppercase;
  color: #ea580c;
}

.tv-summary-card,
.tv-day-row,
.tv-mini-card {
  backdrop-filter: blur(14px);
}

.tv-card-action:hover {
  transform: translateY(-1px);
}

.tv-summary-card {
  border-radius: 18px;
  padding: 1.15rem 1.25rem;
  box-shadow: var(--tv-shadow-soft);
  border: 1px solid rgba(226, 232, 240, 0.95);
}

.tv-summary-card-ink {
  background: #ffffff;
}

.tv-summary-card-ready {
  background: rgba(240, 253, 244, 0.68);
}

.tv-summary-card-today {
  background: rgba(255, 247, 237, 0.78);
}

.tv-summary-card-warning {
  background: rgba(255, 241, 242, 0.78);
}

.tv-summary-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 999px;
  background: rgba(248, 250, 252, 0.95);
  color: #0f172a;
  box-shadow: inset 0 0 0 1px rgba(226, 232, 240, 0.85);
}

.tv-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem 1rem;
}

.tv-picker-inline {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.tv-picker-copy {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.tv-picker-field {
  position: relative;
  width: 100%;
}

.tv-picker-icon {
  position: absolute;
  inset: 0 auto 0 0.75rem;
  display: flex;
  align-items: center;
  color: #9ca3af;
  pointer-events: none;
  z-index: 1;
  transition: color 0.2s ease;
}

.tv-picker-field:focus-within .tv-picker-icon {
  color: #e50914;
}

.tv-toolbar-label {
  font-size: 0.78rem;
  font-weight: 600;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: #64748b;
}

.tv-status-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.9);
  background: #ffffff;
  padding: 0.5rem 0.9rem;
  font-size: 0.78rem;
  font-weight: 600;
  color: #475569;
  transition: transform 180ms ease, border-color 180ms ease, background 180ms ease, color 180ms ease;
  cursor: pointer;
}

.tv-status-chip:hover {
  transform: translateY(-1px);
  border-color: rgba(148, 163, 184, 0.95);
  color: #0f172a;
}

.tv-status-chip-active {
  border-color: rgba(15, 23, 42, 0.9);
  background: rgba(15, 23, 42, 0.94);
  color: #ffffff;
}

.tv-picker-card {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.tv-week-picker {
  width: 200px;
}

.tv-week-picker :deep(.el-input__wrapper) {
  height: 42px;
  min-height: 42px;
  background-color: #f9fafb !important;
  border-radius: 0.75rem;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

.tv-week-picker :deep(.el-input__wrapper:hover) {
  background-color: #ffffff !important;
}

.tv-week-picker :deep(.el-input__wrapper.is-focus) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px #e50914 inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

.tv-week-picker :deep(.el-input__inner) {
  height: 100%;
  padding-left: 2.5rem;
  font-size: 0.875rem;
  color: #111827;
}

.tv-week-picker :deep(.el-input__inner::placeholder) {
  color: #9ca3af;
}

.tv-week-picker :deep(.el-input__prefix),
.tv-week-picker :deep(.el-input__suffix) {
  display: none;
}

.tv-picker-hint {
  font-size: 0.72rem;
  line-height: 1.45;
  color: #64748b;
}

.tv-status-group {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
}
.tv-mini-poster {
  overflow: hidden;
  background: linear-gradient(145deg, #f8d7a3 0%, #fff3db 100%);
}

.tv-day-row {
  display: grid;
  grid-template-columns: 108px minmax(0, 1fr);
  gap: 1rem;
  border-radius: 20px;
  border: 1px solid rgba(226, 232, 240, 0.88);
  background: #ffffff;
  padding: 0.9rem;
  box-shadow: var(--tv-shadow-soft);
}

.tv-day-row-active {
  background: linear-gradient(180deg, rgba(255, 255, 255, 1), rgba(255, 252, 247, 0.96));
}

.tv-day-row-today {
  border-color: rgba(251, 146, 60, 0.45);
  background:
    radial-gradient(circle at top right, rgba(251, 146, 60, 0.08), transparent 32%),
    linear-gradient(180deg, rgba(255, 250, 242, 0.98), rgba(255, 255, 255, 0.98));
}

.tv-day-row-head {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 100%;
  padding-right: 0.25rem;
  border-right: 1px solid rgba(226, 232, 240, 0.8);
}

.tv-day-row-track {
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 0.25rem;
}

.tv-day-row-track::-webkit-scrollbar {
  height: 6px;
}

.tv-day-row-track::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(203, 213, 225, 0.9);
}

.tv-day-row-track::-webkit-scrollbar-track {
  background: transparent;
}

.tv-day-row-strip {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: 136px;
  gap: 0.75rem;
}

.tv-day-row-empty {
  display: flex;
  align-items: center;
  min-height: 140px;
  padding-left: 0.5rem;
}

.tv-mini-card {
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.85);
  background: rgba(255, 255, 255, 0.98);
  padding: 0.55rem;
  box-shadow: 0 6px 14px rgba(15, 23, 42, 0.04);
  transition: transform 180ms ease, box-shadow 180ms ease, border-color 180ms ease;
}

.tv-mini-card:hover {
  transform: translateY(-2px);
  border-color: rgba(148, 163, 184, 0.5);
  box-shadow: 0 12px 24px rgba(15, 23, 42, 0.08);
}

.tv-mini-poster-shell {
  position: relative;
  overflow: hidden;
  border-radius: 14px;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.08), rgba(15, 23, 42, 0.32));
}

.tv-mini-poster {
  width: 100%;
  aspect-ratio: 3 / 4;
  border-radius: 14px;
  flex-shrink: 0;
}

.tv-mini-overlay {
  position: absolute;
  inset: auto 0 0 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.4rem;
  padding: 0.55rem;
  background: linear-gradient(180deg, transparent 0%, rgba(15, 23, 42, 0.78) 100%);
}

.tv-mini-code {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.5);
  padding: 0.22rem 0.5rem;
  font-size: 0.58rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.92);
}

.tv-status-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 24px;
  border-radius: 999px;
  padding: 0.24rem 0.62rem;
  font-size: 0.64rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.tv-status-badge-ready {
  background: rgba(220, 252, 231, 0.95);
  color: #166534;
}

.tv-status-badge-today {
  background: rgba(255, 237, 213, 0.95);
  color: #c2410c;
}

.tv-status-badge-missing {
  background: rgba(254, 226, 226, 0.95);
  color: #b91c1c;
}

.tv-status-badge-upcoming {
  background: rgba(226, 232, 240, 0.95);
  color: #475569;
}

.tv-card-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.9);
  background: rgba(248, 250, 252, 0.95);
  min-width: 46px;
  padding: 0.32rem 0.6rem;
  font-size: 0.66rem;
  font-weight: 700;
  color: #0f172a;
  line-height: 1;
  white-space: nowrap;
  transition: background 180ms ease, border-color 180ms ease, transform 180ms ease;
  cursor: pointer;
}

.tv-card-action-active {
  border-color: rgba(251, 191, 36, 0.42);
  background: rgba(255, 251, 235, 0.95);
  color: #92400e;
}

.tv-floating-alert :deep(.el-alert) {
  border-radius: 24px;
}

.tv-card-action:focus-visible,
.tv-status-chip:focus-visible {
  outline: 2px solid rgba(15, 23, 42, 0.85);
  outline-offset: 2px;
}

@media (max-width: 1279px) {
  .tv-day-row {
    grid-template-columns: 96px minmax(0, 1fr);
  }

  .tv-day-row-strip {
    grid-auto-columns: 128px;
  }
}

@media (max-width: 767px) {
  .tv-stage,
  .tv-panel,
  .tv-day-row {
    border-radius: 18px;
  }

  .tv-day-row {
    grid-template-columns: 1fr;
  }

  .tv-toolbar,
  .tv-picker-card,
  .tv-picker-inline {
    width: 100%;
    align-items: stretch;
    flex-direction: column;
  }

  .tv-picker-copy {
    width: 100%;
  }

  .tv-week-picker {
    width: 100%;
  }

  .tv-day-row-head {
    border-right: 0;
    border-bottom: 1px solid rgba(226, 232, 240, 0.8);
    padding-right: 0;
    padding-bottom: 0.75rem;
  }

  .tv-day-row-strip {
    grid-auto-columns: 132px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .tv-status-chip,
  .tv-card-action {
    transition: none;
  }
}
</style>
