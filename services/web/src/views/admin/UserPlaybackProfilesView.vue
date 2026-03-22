<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshRight, Search, UserFilled } from '@element-plus/icons-vue'
import { getUserPlaybackProfiles } from '@/api/admin'
import { formatPlaybackDate } from '@/utils/date'
import type {
  PlaybackProfileListItem,
  PlaybackProfileListQuery,
  PlaybackProfileListSortBy,
  PlaybackProfileListSortOrder,
  PlaybackProfileRange
} from '@/types/api'

const router = useRouter()
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
  { label: '近 7 天', value: '7d' },
  { label: '近 30 天', value: '30d' },
  { label: '近 90 天', value: '90d' },
  { label: '全部历史', value: 'all' }
]

const queryParams = ref<PlaybackProfileListQuery>({
  range: '30d',
  keyword: '',
  sortBy: 'totalDuration',
  sortOrder: 'desc',
  page: 1,
  pageSize: 20
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getUserPlaybackProfiles({
      ...queryParams.value,
      keyword: queryParams.value.keyword?.trim() || undefined
    })
    tableData.value = res.data
    total.value = res.total
    summary.value = res.summary
  } finally {
    loading.value = false
  }
}

const buildDateString = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const buildRangeQuery = () => {
  if (queryParams.value.range === 'all') return {}

  const end = new Date()
  const start = new Date()
  if (queryParams.value.range === '7d') start.setDate(start.getDate() - 7)
  if (queryParams.value.range === '30d') start.setDate(start.getDate() - 30)
  if (queryParams.value.range === '90d') start.setDate(start.getDate() - 90)

  return {
    startDate: buildDateString(start),
    endDate: buildDateString(end)
  }
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleReset = () => {
  queryParams.value = {
    range: '30d',
    keyword: '',
    sortBy: 'totalDuration',
    sortOrder: 'desc',
    page: 1,
    pageSize: 20
  }
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
    query: { range: queryParams.value.range }
  })
}

const handleViewHistory = (row: PlaybackProfileListItem) => {
  router.push({
    name: 'console-playback-history',
    query: {
      username: row.username,
      ...buildRangeQuery()
    }
  })
}

watch(
  () => [queryParams.value.range, queryParams.value.sortBy],
  () => {
    queryParams.value.page = 1
    fetchData()
  }
)

onMounted(() => {
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
            用户画像总览
            <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Total: {{ total }}</span>
          </h1>
          <p class="text-gray-500 text-sm mt-1">按用户聚合查看播放活跃度，先发现重点用户，再进入画像和历史明细</p>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 xl:min-w-[30rem]">
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-4">
            <p class="text-xs font-medium text-gray-500">有播放用户</p>
            <p class="mt-2 text-2xl font-bold text-gray-900">{{ summary.userCount }}</p>
          </div>
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-4">
            <p class="text-xs font-medium text-gray-500">总播放次数</p>
            <p class="mt-2 text-2xl font-bold text-gray-900">{{ summary.totalPlayCount }}</p>
          </div>
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-4">
            <p class="text-xs font-medium text-gray-500">总播放时长</p>
            <p class="mt-2 text-2xl font-bold text-gray-900">{{ summary.totalPlayDurationFormatted }}</p>
          </div>
        </div>
      </div>

      <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap gap-2">
            <button
              v-for="option in rangeOptions"
              :key="option.value"
              @click="queryParams.range = option.value"
              class="px-4 py-2 text-sm rounded-xl border transition-colors cursor-pointer"
              :class="queryParams.range === option.value
                ? 'border-ember bg-ember text-white'
                : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'"
            >
              {{ option.label }}
            </button>
          </div>

          <div class="flex flex-col xl:flex-row xl:items-end gap-3">
            <div class="w-full xl:max-w-sm space-y-1.5">
              <div class="space-y-1.5">
                <label class="text-xs font-semibold tracking-wide text-gray-500">用户名</label>
                <div class="relative group">
                  <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                    <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><UserFilled /></el-icon>
                  </div>
                  <input
                    v-model="queryParams.keyword"
                    type="search"
                    autocomplete="off"
                    aria-label="按用户名筛选"
                    placeholder="输入用户名筛选"
                    class="filter-input w-full pl-10 pr-4"
                    @keyup.enter="handleSearch"
                  />
                </div>
              </div>
            </div>

            <div class="flex items-end justify-end gap-2 xl:ml-auto xl:shrink-0">
              <button
                @click="handleReset"
                class="px-4 py-2.5 text-sm text-gray-700 bg-white border border-gray-200 hover:bg-gray-100 rounded-xl transition-colors cursor-pointer inline-flex items-center gap-1.5"
              >
                <el-icon><RefreshRight /></el-icon>
                重置
              </button>
              <button
                @click="handleSearch"
                class="btn-ember px-4 py-2.5 text-sm rounded-xl font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer inline-flex items-center gap-1.5"
              >
                <el-icon><Search /></el-icon>
                查询
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
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
      </el-table>

      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
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
      </div>
    </div>
  </div>
</template>

<style scoped>
.filter-input {
  background-color: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 0.75rem;
  height: 42px;
  line-height: 1.2;
  font-size: 0.875rem;
  color: #111827;
  outline: none;
  transition: all 0.2s ease;
}

.filter-input::placeholder {
  color: #9ca3af;
}

.filter-input:hover {
  background-color: #ffffff;
}

.filter-input:focus {
  background-color: #ffffff;
  border-color: var(--ember-red);
  box-shadow: 0 0 0 4px rgba(229, 9, 20, 0.1);
}

</style>
