<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Star } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import {
  getFollowingTVCalendar,
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
  TVCalendarWeeklyItem,
  TVCalendarWeekOffset
} from '@/types/api'

type CalendarMode = 'global' | 'following'

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)

const loading = ref(false)
const refreshing = ref(false)
const subscriptionsLoading = ref(false)
const calendarError = ref('')
const subscriptionsError = ref('')

const subscriptions = ref<TVCalendarSubscription[]>([])
const calendarData = ref<TVCalendarWeeklyData>({ dateRange: '', days: [] })

const filters = reactive({
  mode: 'global' as CalendarMode,
  weekOffset: 0 as TVCalendarWeekOffset,
  status: '' as TVCalendarStatus | ''
})

const weekOptions: Array<{ label: string; value: TVCalendarWeekOffset }> = [
  { label: '上周', value: -1 },
  { label: '本周', value: 0 },
  { label: '下周', value: 1 }
]

const viewOptions: Array<{ label: string; value: CalendarMode }> = [
  { label: '全部在更', value: 'global' },
  { label: '我的关注', value: 'following' }
]

const statusOptions: Array<{ label: string; value: TVCalendarStatus | '' }> = [
  { label: '全部状态', value: '' },
  { label: '已入库', value: 'ready' },
  { label: '缺失', value: 'missing' },
  { label: '今日播出', value: 'today' },
  { label: '待播', value: 'upcoming' }
]

const subscriptionSet = computed(() => new Set(subscriptions.value.map((item) => item.tmdbId)))
const dayColumns = computed(() => calendarData.value.days || [])
const hasCalendarItems = computed(() => dayColumns.value.some((day) => day.items.length > 0))
const currentRange = computed(() => calendarData.value.dateRange || '--')
const currentViewDescription = computed(() => {
  if (filters.mode === 'global') {
    return '默认展示 Emby 中识别到的连载剧周历，不再要求先手动关注。'
  }
  return '只看你已经关注的剧集，适合收窄视图，不适合作为默认入口。'
})
const emptyStateTitle = computed(() => {
  if (filters.mode === 'following') {
    return subscriptions.value.length === 0 ? '你还没有关注任何剧集' : '本周关注列表里没有匹配条目'
  }
  return '当前服务器中没有识别到正在连载的剧集'
})
const emptyStateDescription = computed(() => {
  if (filters.mode === 'following') {
    if (subscriptions.value.length === 0) {
      return '先从“全部在更”里挑你关心的剧，再切回来过滤。'
    }
    return '这个周区间或当前状态筛选下没有命中结果。'
  }
  return '检查 Emby 连载剧识别、TMDB 配置，或者让管理员手动同步一次。'
})

function extractErrorMessage(error: unknown, fallback: string): string {
  const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
  if (typeof message === 'string' && message.trim()) {
    return message.trim()
  }
  return fallback
}

function statusTagType(status: TVCalendarStatus): 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'ready':
      return 'success'
    case 'today':
      return 'warning'
    case 'missing':
      return 'danger'
    default:
      return 'info'
  }
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

async function fetchSubscriptions(): Promise<void> {
  subscriptionsLoading.value = true
  subscriptionsError.value = ''
  try {
    const res = await getTVCalendarSubscriptions()
    subscriptions.value = res.data || []
  } catch (error) {
    subscriptions.value = []
    subscriptionsError.value = extractErrorMessage(error, '读取关注列表失败')
  } finally {
    subscriptionsLoading.value = false
  }
}

async function fetchCalendar(): Promise<void> {
  loading.value = true
  calendarError.value = ''
  try {
    const params = {
      weekOffset: filters.weekOffset,
      status: filters.status || undefined
    }
    const res =
      filters.mode === 'following'
        ? await getFollowingTVCalendar(params)
        : await getGlobalTVCalendar(params)
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

async function changeMode(mode: CalendarMode): Promise<void> {
  filters.mode = mode
  await fetchCalendar()
}

async function changeWeek(weekOffset: TVCalendarWeekOffset): Promise<void> {
  filters.weekOffset = weekOffset
  await fetchCalendar()
}

async function changeStatus(status: TVCalendarStatus | ''): Promise<void> {
  filters.status = status
  await fetchCalendar()
}

onMounted(() => {
  void loadAll()
})
</script>

<template>
  <div class="space-y-6">
    <section class="overflow-hidden rounded-[28px] border border-amber-100 bg-[radial-gradient(circle_at_top_left,_rgba(251,191,36,0.28),_transparent_38%),linear-gradient(135deg,#fff7ed_0%,#ffffff_60%,#fef3c7_100%)] p-6 shadow-sm">
      <div class="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
        <div class="max-w-3xl">
          <p class="text-xs font-semibold uppercase tracking-[0.35em] text-amber-600">TV Calendar</p>
          <h1 class="mt-3 text-3xl font-semibold text-slate-900">追剧日历</h1>
          <p class="mt-3 text-sm leading-6 text-slate-600">{{ currentViewDescription }}</p>
          <div class="mt-5 flex flex-wrap gap-2">
            <el-radio-group :model-value="filters.mode" @change="changeMode">
              <el-radio-button
                v-for="option in viewOptions"
                :key="option.value"
                :label="option.value"
              >
                {{ option.label }}
              </el-radio-button>
            </el-radio-group>
            <el-radio-group :model-value="filters.weekOffset" @change="changeWeek">
              <el-radio-button
                v-for="option in weekOptions"
                :key="option.value"
                :label="option.value"
              >
                {{ option.label }}
              </el-radio-button>
            </el-radio-group>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <div class="rounded-2xl border border-white/70 bg-white/75 px-4 py-3 shadow-sm backdrop-blur">
            <p class="text-xs uppercase tracking-[0.25em] text-slate-400">周区间</p>
            <p class="mt-1 text-lg font-semibold text-slate-900">{{ currentRange }}</p>
          </div>
          <el-select
            :model-value="filters.status"
            class="!w-[160px]"
            placeholder="状态筛选"
            @change="changeStatus"
          >
            <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
          <el-button v-if="isAdmin" :icon="Refresh" :loading="refreshing" @click="handleSync">手动同步</el-button>
        </div>
      </div>

      <el-alert
        v-if="calendarError"
        class="mt-5"
        type="warning"
        :closable="false"
        show-icon
        title="追剧日历暂时不可用"
        :description="calendarError"
      />
    </section>

    <section class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-slate-900">我的关注</h2>
        </div>
        <div class="flex items-center gap-2 text-sm text-slate-500">
          <el-icon><Star /></el-icon>
          <span>{{ subscriptions.length }} 部剧</span>
        </div>
      </div>

      <el-alert
        v-if="subscriptionsError"
        class="mt-4"
        type="warning"
        :closable="false"
        show-icon
        title="关注列表读取失败"
        :description="subscriptionsError"
      />

      <div v-else-if="subscriptionsLoading" class="mt-4 text-sm text-slate-400">正在读取关注列表...</div>

      <div
        v-else-if="subscriptions.length === 0"
        class="mt-4 rounded-2xl border border-dashed border-slate-200 bg-slate-50 px-4 py-6 text-sm text-slate-500"
      >
        当前没有任何关注剧集。先从“全部在更”里挑剧，再切到“我的关注”。
      </div>

      <div v-else class="mt-4 flex flex-wrap gap-2">
        <el-tag
          v-for="subscription in subscriptions"
          :key="subscription.id"
          effect="plain"
          type="warning"
          class="!px-3 !py-2"
        >
          <button class="text-sm text-slate-700" @click="handleUnfollow(subscription.tmdbId, subscription.showName)">
            {{ subscription.showName }}
          </button>
        </el-tag>
      </div>
    </section>

    <section class="rounded-3xl border border-slate-200 bg-white p-5 shadow-sm">
      <div v-if="loading" class="py-20 text-center text-sm text-slate-400">正在读取周历...</div>

      <div
        v-else-if="!hasCalendarItems"
        class="rounded-2xl border border-dashed border-slate-200 bg-slate-50 px-6 py-14 text-center"
      >
        <p class="text-base font-semibold text-slate-800">{{ emptyStateTitle }}</p>
        <p class="mt-2 text-sm text-slate-500">{{ emptyStateDescription }}</p>
        <div class="mt-5 flex justify-center gap-3">
          <el-button
            v-if="filters.mode === 'following'"
            type="primary"
            plain
            @click="changeMode('global')"
          >
            去看全部在更
          </el-button>
          <el-button v-if="isAdmin" :icon="Refresh" @click="handleSync">立即同步</el-button>
        </div>
      </div>

      <div v-else class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-7">
        <article
          v-for="day in dayColumns"
          :key="day.date"
          class="min-h-[320px] rounded-2xl border border-slate-200 bg-slate-50/80 p-4"
        >
          <div class="flex items-start justify-between border-b border-slate-200 pb-3">
            <div>
              <p class="text-sm font-semibold text-slate-900">{{ day.weekdayCn }}</p>
              <p class="mt-1 text-xs text-slate-500">{{ day.date }}</p>
            </div>
            <span
              class="rounded-full px-2.5 py-1 text-xs font-medium"
              :class="day.isToday ? 'bg-amber-500 text-white' : 'bg-white text-slate-500'"
            >
              {{ day.isToday ? '今天' : `${day.items.length} 集` }}
            </span>
          </div>

          <div v-if="day.items.length === 0" class="flex min-h-[220px] items-center justify-center text-center text-sm text-slate-400">
            当天没有匹配条目
          </div>

          <div v-else class="mt-4 space-y-3">
            <div
              v-for="item in day.items"
              :key="`${day.date}-${item.tmdbId}-${item.season}-${item.episode}`"
              class="rounded-2xl border border-white bg-white p-3 shadow-sm"
            >
              <div class="flex gap-3">
                <div class="h-20 w-14 shrink-0 overflow-hidden rounded-xl bg-gradient-to-br from-amber-300 via-orange-200 to-yellow-100">
                  <img
                    v-if="item.posterUrl"
                    :src="item.posterUrl"
                    :alt="item.showName"
                    class="h-full w-full object-cover"
                  />
                  <div v-else class="flex h-full w-full items-center justify-center text-lg font-semibold text-amber-800">
                    {{ getPosterFallback(item.showName) }}
                  </div>
                </div>

                <div class="min-w-0 flex-1">
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0">
                      <p class="truncate text-sm font-semibold text-slate-900">{{ item.showName }}</p>
                      <p class="mt-1 text-xs text-slate-500">S{{ String(item.season).padStart(2, '0') }} · E{{ item.episode }}</p>
                    </div>
                    <el-tag :type="statusTagType(item.status)" size="small">{{ statusText(item.status) }}</el-tag>
                  </div>

                  <p v-if="item.episodeName" class="mt-2 text-sm text-slate-700">{{ item.episodeName }}</p>
                  <p v-if="item.overview" class="mt-2 max-h-16 overflow-hidden text-xs leading-5 text-slate-500">
                    {{ item.overview }}
                  </p>

                  <div class="mt-3 flex items-center justify-between gap-2">
                    <span class="text-xs text-slate-400">{{ item.airDate }}</span>
                    <el-button
                      v-if="filters.mode === 'global'"
                      size="small"
                      :type="isFollowing(item.tmdbId) ? 'info' : 'primary'"
                      plain
                      @click="handleFollow(item)"
                    >
                      {{ isFollowing(item.tmdbId) ? '取消关注' : '加入关注' }}
                    </el-button>
                    <el-button
                      v-else
                      size="small"
                      type="info"
                      plain
                      @click="handleUnfollow(item.tmdbId, item.showName)"
                    >
                      取消关注
                    </el-button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>
