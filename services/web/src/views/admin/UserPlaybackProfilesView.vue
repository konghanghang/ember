<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Calendar, RefreshRight, Search, UserFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getUserPlaybackProfiles } from '@/api/admin'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberDateRangeField from '@/components/ember/filters/EmberDateRangeField.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { emberRangePickerPopperClass, rangePickerDefaultTime } from '@/constants/datePicker'
import { formatPlaybackDate } from '@/utils/date'
import type {
  PlaybackProfileListItem,
  PlaybackProfileListQuery,
  PlaybackProfileListSortBy,
  PlaybackProfileListSortOrder,
  PlaybackProfileRange
} from '@/types/api'

const router = useRouter()
const MAX_CUSTOM_RANGE_DAYS = 92
const loading = ref(false)
const tableData = ref<PlaybackProfileListItem[]>([])
const total = ref(0)
const summary = ref({
  userCount: 0,
  totalPlayCount: 0,
  totalPlayDuration: 0,
  totalPlayDurationFormatted: '0m'
})

const rangeOptions: Array<{ label: string; value: PlaybackProfileRange }> = [
  { label: '当天', value: 'today' },
  { label: '近 7 天', value: '7d' },
  { label: '近 30 天', value: '30d' },
  { label: '近 90 天', value: '90d' },
  { label: '全部历史', value: 'all' }
]

const createDefaultQueryParams = (): PlaybackProfileListQuery => ({
  range: 'today',
  keyword: '',
  sortBy: 'totalDuration',
  sortOrder: 'desc',
  page: 1,
  pageSize: 20
})

const queryParams = ref<PlaybackProfileListQuery>(createDefaultQueryParams())
const customDateRange = ref<[string, string] | null>(null)
const rangeAnchorDate = ref<Date | null>(null)
const isCustomRange = computed(() => queryParams.value.range === 'custom')

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getUserPlaybackProfiles({
      ...queryParams.value,
      keyword: queryParams.value.keyword?.trim() || undefined,
      startDate: isCustomRange.value ? queryParams.value.startDate : undefined,
      endDate: isCustomRange.value ? queryParams.value.endDate : undefined
    })
    tableData.value = res.data
    total.value = res.total
    summary.value = res.summary
  } finally {
    loading.value = false
  }
}

const buildDateTimeString = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  const second = String(date.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}:${second}`
}

const buildPresetDateRange = (range: PlaybackProfileRange): [string, string] | null => {
  if (range === 'all' || range === 'custom') return null

  const end = new Date()
  const start = new Date()
  if (range === 'today') {
    start.setHours(0, 0, 0, 0)
  }
  if (range === '7d') start.setDate(start.getDate() - 7)
  if (range === '30d') start.setDate(start.getDate() - 30)
  if (range === '90d') start.setDate(start.getDate() - 90)

  return [buildDateTimeString(start), buildDateTimeString(end)]
}

const syncCustomDateRange = () => {
  if (isCustomRange.value && queryParams.value.startDate && queryParams.value.endDate) {
    customDateRange.value = [queryParams.value.startDate, queryParams.value.endDate]
    return
  }
  customDateRange.value = buildPresetDateRange(queryParams.value.range ?? 'today')
}

const toDateOnly = (value: string) => value.slice(0, 10)

const buildRangeQuery = () => {
  if (isCustomRange.value && queryParams.value.startDate && queryParams.value.endDate) {
    return {
      range: 'custom' as const,
      startDate: queryParams.value.startDate,
      endDate: queryParams.value.endDate
    }
  }
  return {
    range: queryParams.value.range
  }
}

const buildHistoryRangeQuery = () => {
  if (isCustomRange.value && queryParams.value.startDate && queryParams.value.endDate) {
    return {
      startDate: toDateOnly(queryParams.value.startDate),
      endDate: toDateOnly(queryParams.value.endDate)
    }
  }

  const presetRange = buildPresetDateRange(queryParams.value.range ?? 'today')
  if (!presetRange) return {}

  return {
    startDate: toDateOnly(presetRange[0]),
    endDate: toDateOnly(presetRange[1])
  }
}

const isRangeTooLarge = (startDate: string, endDate: string) => {
  const start = new Date(startDate.replace(' ', 'T'))
  const end = new Date(endDate.replace(' ', 'T'))
  return end.getTime() - start.getTime() > MAX_CUSTOM_RANGE_DAYS * 24 * 60 * 60 * 1000
}

const disabledCustomDate = (date: Date) => {
  if (date.getTime() > Date.now()) {
    return true
  }
  if (!rangeAnchorDate.value) {
    return false
  }
  return Math.abs(date.getTime() - rangeAnchorDate.value.getTime()) > MAX_CUSTOM_RANGE_DAYS * 24 * 60 * 60 * 1000
}

const handleCustomCalendarChange = (value: Date[]) => {
  rangeAnchorDate.value = value?.[0] ?? null
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleRangeChange = (range: PlaybackProfileRange) => {
  rangeAnchorDate.value = null
  queryParams.value = {
    ...queryParams.value,
    range,
    startDate: undefined,
    endDate: undefined,
    page: 1
  }
  customDateRange.value = buildPresetDateRange(range)
  fetchData()
}

const handleCustomRangeChange = (value: [string, string] | null) => {
  rangeAnchorDate.value = null

  if (!value || value.length !== 2) {
    const fallbackRange: PlaybackProfileRange = 'today'
    queryParams.value = {
      ...queryParams.value,
      range: fallbackRange,
      startDate: undefined,
      endDate: undefined,
      page: 1
    }
    customDateRange.value = buildPresetDateRange(fallbackRange)
    fetchData()
    return
  }

  if (isRangeTooLarge(value[0], value[1])) {
    ElMessage.warning('自定义日期范围最多选择 3 个月')
    return
  }

  customDateRange.value = value
  queryParams.value = {
    ...queryParams.value,
    range: 'custom',
    startDate: value[0],
    endDate: value[1],
    page: 1
  }
  fetchData()
}

const handleReset = () => {
  rangeAnchorDate.value = null
  queryParams.value = createDefaultQueryParams()
  syncCustomDateRange()
  fetchData()
}

const isSortActive = (sortBy: PlaybackProfileListSortBy) => queryParams.value.sortBy === sortBy
const sortIndicator = (sortBy: PlaybackProfileListSortBy) => {
  if (!isSortActive(sortBy)) return '↕'
  return queryParams.value.sortOrder === 'desc' ? '↓' : '↑'
}

const handleSort = (sortBy: PlaybackProfileListSortBy) => {
  if (queryParams.value.sortBy === sortBy) {
    queryParams.value.sortOrder = queryParams.value.sortOrder === 'desc' ? 'asc' : 'desc'
  } else {
    queryParams.value.sortBy = sortBy
    queryParams.value.sortOrder = 'desc'
  }
  queryParams.value.page = 1
  fetchData()
}

const handleViewProfile = (row: PlaybackProfileListItem) => {
  router.push({
    name: 'console-user-profile',
    params: { id: row.userId },
    query: buildRangeQuery()
  })
}

const handleViewHistory = (row: PlaybackProfileListItem) => {
  router.push({
    name: 'console-playback-history',
    query: {
      username: row.username,
      ...buildHistoryRangeQuery()
    }
  })
}

onMounted(() => {
  syncCustomDateRange()
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="用户画像总览"
      description="按用户聚合查看播放活跃度，先发现重点用户，再进入画像和历史明细"
    >
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">Total: {{ total }}</span>
      </template>

      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3 xl:min-w-[30rem]">
        <EmberMetricCard title="有播放用户" :value="summary.userCount" />
        <EmberMetricCard title="总播放次数" :value="summary.totalPlayCount" />
        <EmberMetricCard title="总播放时长" :value="summary.totalPlayDurationFormatted" />
      </div>

      <EmberFilterPanel
        wrapper-class="flex flex-col gap-3"
        content-class="flex flex-col gap-3"
        actions-class="hidden"
      >
        <div class="flex flex-col gap-3 xl:flex-row xl:items-end">
          <div class="flex flex-wrap gap-2 xl:shrink-0">
            <button
              v-for="option in rangeOptions"
              :key="option.value"
              @click="handleRangeChange(option.value)"
              class="cursor-pointer rounded-xl border px-4 py-2 text-sm transition-colors"
              :class="queryParams.range === option.value
                ? 'border-ember bg-ember text-white'
                : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'"
            >
              {{ option.label }}
            </button>
          </div>

          <div class="min-w-0 flex-1 xl:max-w-[34rem]">
            <EmberDateRangeField
              v-model="customDateRange"
              label="日期范围"
              type="datetimerange"
              start-placeholder="开始日期时间"
              end-placeholder="结束日期时间"
              value-format="YYYY-MM-DD HH:mm:ss"
              :default-time="rangePickerDefaultTime"
              :popper-class="emberRangePickerPopperClass"
              unlink-panels
              clearable
              :disabled-date="disabledCustomDate"
              :icon="Calendar"
              @calendar-change="handleCustomCalendarChange"
              @change="handleCustomRangeChange"
            />
          </div>
        </div>

        <div class="flex flex-col gap-3 xl:flex-row xl:items-end">
          <div class="w-full xl:max-w-xs">
            <EmberSearchInput
              v-model="queryParams.keyword"
              label="用户名"
              aria-label="按用户名筛选"
              placeholder="输入用户名筛选"
              :icon="UserFilled"
              @enter="handleSearch"
            />
          </div>

          <div class="flex items-end justify-end gap-2 xl:ml-auto xl:shrink-0">
            <button
              @click="handleReset"
              class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
            >
              <el-icon><RefreshRight /></el-icon>
              重置
            </button>
            <button
              @click="handleSearch"
              class="btn-ember inline-flex cursor-pointer items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
            >
              <el-icon><Search /></el-icon>
              查询
            </button>
          </div>
        </div>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
        <el-table-column prop="username" label="用户" min-width="140" />
        <el-table-column width="130">
          <template #header>
            <button
              @click="handleSort('totalDuration')"
              class="inline-flex items-center gap-1 text-sm font-semibold transition-colors cursor-pointer"
              :class="isSortActive('totalDuration') ? 'text-ember' : 'text-gray-500 hover:text-gray-700'"
            >
              <span>累计时长</span>
              <span class="text-xs">{{ sortIndicator('totalDuration') }}</span>
            </button>
          </template>
          <template #default="{ row }">
            {{ row.totalPlayDurationFormatted }}
          </template>
        </el-table-column>
        <el-table-column width="110">
          <template #header>
            <button
              @click="handleSort('playCount')"
              class="inline-flex items-center gap-1 text-sm font-semibold transition-colors cursor-pointer"
              :class="isSortActive('playCount') ? 'text-ember' : 'text-gray-500 hover:text-gray-700'"
            >
              <span>播放次数</span>
              <span class="text-xs">{{ sortIndicator('playCount') }}</span>
            </button>
          </template>
          <template #default="{ row }">
            {{ row.totalPlayCount }}
          </template>
        </el-table-column>
        <el-table-column width="110">
          <template #header>
            <button
              @click="handleSort('activeDays')"
              class="inline-flex items-center gap-1 text-sm font-semibold transition-colors cursor-pointer"
              :class="isSortActive('activeDays') ? 'text-ember' : 'text-gray-500 hover:text-gray-700'"
            >
              <span>活跃天数</span>
              <span class="text-xs">{{ sortIndicator('activeDays') }}</span>
            </button>
          </template>
          <template #default="{ row }">
            {{ row.activeDays }}
          </template>
        </el-table-column>
        <el-table-column min-width="170">
          <template #header>
            <button
              @click="handleSort('lastPlayedAt')"
              class="inline-flex items-center gap-1 text-sm font-semibold transition-colors cursor-pointer"
              :class="isSortActive('lastPlayedAt') ? 'text-ember' : 'text-gray-500 hover:text-gray-700'"
            >
              <span>最近播放</span>
              <span class="text-xs">{{ sortIndicator('lastPlayedAt') }}</span>
            </button>
          </template>
          <template #default="{ row }">
            {{ row.lastPlayedAt ? formatPlaybackDate(row.lastPlayedAt) : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="峰值时段" min-width="140">
          <template #default="{ row }">
            <span class="inline-flex rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700">
              {{ row.peakHourLabel || '-' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="标签摘要" min-width="220">
          <template #default="{ row }">
            <div v-if="row.badges?.length" class="flex flex-wrap gap-2">
              <span
                v-for="badge in row.badges"
                :key="badge.id"
                class="inline-flex rounded-full bg-ember/10 px-2.5 py-1 text-xs font-medium text-ember"
              >
                {{ badge.name }}
              </span>
            </div>
            <span v-else class="text-sm text-gray-400">暂无标签</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <div class="flex items-center justify-end gap-3">
              <button
                @click="handleViewProfile(row)"
                class="text-sm font-medium text-ember hover:text-ember/80 transition-colors cursor-pointer"
              >
                查看画像
              </button>
              <button
                @click="handleViewHistory(row)"
                class="text-sm font-medium text-gray-600 hover:text-gray-900 transition-colors cursor-pointer"
              >
                播放历史
              </button>
            </div>
          </template>
        </el-table-column>

      <template #pagination>
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="fetchData"
          @size-change="fetchData"
          background
        />
      </template>
    </EmberTableCard>
  </div>
</template>
