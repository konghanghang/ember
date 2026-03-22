<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, RefreshRight, UserFilled, Calendar } from '@element-plus/icons-vue'
import { getPlaybackHistory } from '@/api/admin'
import { formatPlaybackDate } from '@/utils/date'
import type { PlaybackHistoryItem, PlaybackHistoryQuery } from '@/types/api'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const tableData = ref<PlaybackHistoryItem[]>([])
const total = ref(0)
const dateRange = ref<[string, string] | null>(null)

const queryParams = ref<PlaybackHistoryQuery>({
  page: 1,
  pageSize: 20,
  username: '',
  userId: '',
  keyword: ''
})

const fetchData = async () => {
  loading.value = true
  try {
    const params: PlaybackHistoryQuery = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize,
      username: queryParams.value.username?.trim() || undefined,
      userId: queryParams.value.username?.trim() ? undefined : queryParams.value.userId?.trim() || undefined,
      keyword: queryParams.value.keyword?.trim() || undefined
    }

    if (dateRange.value && dateRange.value.length === 2) {
      params.startDate = dateRange.value[0]
      params.endDate = dateRange.value[1]
    }

    const res = await getPlaybackHistory(params)
    tableData.value = res.data
    total.value = res.total
  } catch {
    // 全局错误拦截已处理
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleReset = () => {
  queryParams.value.page = 1
  queryParams.value.pageSize = 20
  queryParams.value.username = ''
  queryParams.value.userId = ''
  queryParams.value.keyword = ''
  dateRange.value = null
  fetchData()
}

const handlePageSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const syncQueryFromRoute = () => {
  const username = String(route.query.username ?? '').trim()
  if (username) {
    queryParams.value.username = username
    queryParams.value.userId = ''
    return
  }

  const userId = String(route.query.userId ?? '').trim()
  if (userId) {
    queryParams.value.userId = userId
  }
}

const handleViewProfile = (row: PlaybackHistoryItem) => {
  router.push({
    name: 'console-user-profile',
    params: { id: row.userId },
    query: { range: '30d' }
  })
}

onMounted(() => {
  syncQueryFromRoute()
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          播放历史
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Total: {{ total }}</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">按用户、关键词和日期范围筛选播放记录</p>
      </div>

      <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
        <div class="grid grid-cols-1 xl:grid-cols-4 gap-3">
          <div class="space-y-1.5">
            <label class="text-xs font-semibold tracking-wide text-gray-500">用户名</label>
            <div class="relative group">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><UserFilled /></el-icon>
              </div>
              <input
                v-model="queryParams.username"
                type="text"
                autocomplete="off"
                aria-label="按用户名筛选"
                placeholder="输入用户名筛选"
                class="filter-input w-full pl-10 pr-4"
                @keyup.enter="handleSearch"
              />
            </div>
          </div>

          <div class="space-y-1.5">
            <label class="text-xs font-semibold tracking-wide text-gray-500">关键词</label>
            <div class="relative group">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Search /></el-icon>
              </div>
              <input
                v-model="queryParams.keyword"
                type="text"
                autocomplete="off"
                aria-label="按关键词筛选"
                placeholder="片名/设备/客户端"
                class="filter-input w-full pl-10 pr-4"
                @keyup.enter="handleSearch"
              />
            </div>
          </div>

          <div class="space-y-1.5">
            <label class="text-xs font-semibold tracking-wide text-gray-500">日期范围</label>
            <div class="relative w-full group">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none z-10">
                <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Calendar /></el-icon>
              </div>
              <el-date-picker
                v-model="dateRange"
                type="daterange"
                start-placeholder="开始日期"
                end-placeholder="结束日期"
                value-format="YYYY-MM-DD"
                class="w-full filter-date filter-date-range"
                unlink-panels
              />
            </div>
          </div>

          <div class="flex items-end justify-end gap-2">
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

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
        <el-table-column prop="playedAt" label="播放时间" width="180">
          <template #default="{ row }">
            {{ row.playedAt ? formatPlaybackDate(row.playedAt) : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户" width="120" />
        <el-table-column prop="itemName" label="片名" min-width="220" />
        <el-table-column prop="itemType" label="类型" width="110" />
        <el-table-column prop="deviceName" label="设备" min-width="140" />
        <el-table-column prop="clientName" label="客户端" min-width="140" />
        <el-table-column prop="playDurationFormatted" label="时长" width="100" />
        <el-table-column prop="userId" label="用户 ID" min-width="190">
          <template #default="{ row }">
            <code class="text-xs text-gray-600">{{ row.userId || '-' }}</code>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <button
              @click="handleViewProfile(row)"
              class="text-sm font-medium text-ember hover:text-ember/80 transition-colors cursor-pointer"
            >
              用户画像
            </button>
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
          @size-change="handlePageSizeChange"
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

:deep(.filter-date .el-input__wrapper) {
  height: 42px;
  min-height: 42px;
  background-color: #f9fafb !important;
  border-radius: 0.75rem;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.filter-date:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.filter-date .el-input__wrapper.is-focus) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.filter-date .el-input__inner) {
  height: 100%;
  font-size: 0.875rem;
}

:deep(.filter-date-range .el-range__icon) {
  opacity: 0;
  width: 0;
  margin: 0;
}

:deep(.filter-date-range.el-date-editor),
:deep(.filter-date-range .el-range-editor.el-input__wrapper),
:deep(.filter-date-range.el-range-editor.el-input__wrapper) {
  height: 42px !important;
  min-height: 42px !important;
  border-radius: 0.75rem !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  background-color: #f9fafb !important;
}

:deep(.filter-date-range .el-input__wrapper),
:deep(.filter-date-range.el-input__wrapper) {
  overflow: hidden;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  background-color: #f9fafb !important;
}

:deep(.filter-date-range:hover),
:deep(.filter-date-range:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.filter-date-range.is-active),
:deep(.filter-date-range.is-active .el-input__wrapper) {
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
  background-color: #ffffff !important;
}

:deep(.filter-date-range .el-range-input) {
  font-size: 0.875rem;
  background-color: transparent;
}

:deep(.filter-date-range .el-input__wrapper) {
  padding-left: 2.5rem;
}
</style>
