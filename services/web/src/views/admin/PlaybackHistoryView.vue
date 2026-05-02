<script setup lang="ts">
import { onMounted, ref, useSlots } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, RefreshRight, UserFilled, Calendar } from '@element-plus/icons-vue'
import { getPlaybackHistory } from '@/api/admin'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberDateRangeField from '@/components/ember/filters/EmberDateRangeField.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { emberRangePickerPopperClass, rangePickerDefaultTime } from '@/constants/datePicker'
import { formatPlaybackDate } from '@/utils/date'
import type { PlaybackHistoryItem, PlaybackHistoryQuery } from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

const route = useRoute()
const router = useRouter()
const slots = useSlots()
const loading = ref(false)
const tableData = ref<PlaybackHistoryItem[]>([])
const total = ref(0)
const dateRange = ref<[string, string] | null>(null)
let fetchRequestToken = 0

const queryParams = ref<PlaybackHistoryQuery>({
  page: 1,
  pageSize: 20,
  username: '',
  userId: '',
  keyword: ''
})

const fetchData = async () => {
  const requestToken = ++fetchRequestToken
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
    if (requestToken !== fetchRequestToken) {
      return
    }
    tableData.value = res.data
    total.value = res.total
  } catch {
    // 全局错误拦截已处理
  } finally {
    if (requestToken === fetchRequestToken) {
      loading.value = false
    }
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
  }

  const userId = String(route.query.userId ?? '').trim()
  if (userId) {
    queryParams.value.userId = userId
  }

  const startDate = String(route.query.startDate ?? '').trim()
  const endDate = String(route.query.endDate ?? '').trim()
  if (startDate && endDate) {
    dateRange.value = [startDate, endDate]
  }
}

const handleViewProfile = (row: PlaybackHistoryItem) => {
  router.push({
    name: 'console-user-profile',
    params: { id: row.userId },
    query: { range: 'today' }
  })
}

onMounted(() => {
  syncQueryFromRoute()
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="播放历史"
      description="按用户、关键词和日期范围筛选播放记录"
    >
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">
          {{ props.embedded ? `当前结果 ${total} 条` : `Total: ${total}` }}
        </span>
      </template>

      <template v-if="props.embedded && slots.tabs" #actions>
        <slot name="tabs" />
      </template>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-3 xl:grid-cols-4"
        content-class="grid grid-cols-1 gap-3 xl:col-span-3 md:grid-cols-2 xl:grid-cols-3"
        actions-class="flex items-end justify-end gap-2"
      >
        <EmberSearchInput
          v-model="queryParams.username"
          label="用户名"
          aria-label="按用户名筛选"
          placeholder="输入用户名筛选"
          :icon="UserFilled"
          type="text"
          inputmode="text"
          @enter="handleSearch"
        />

        <EmberSearchInput
          v-model="queryParams.keyword"
          label="关键词"
          aria-label="按关键词筛选"
          placeholder="片名/设备/客户端"
          :icon="Search"
          type="text"
          inputmode="text"
          @enter="handleSearch"
        />

        <EmberDateRangeField
          v-model="dateRange"
          label="日期范围"
          type="datetimerange"
          start-placeholder="开始日期时间"
          end-placeholder="结束日期时间"
          value-format="YYYY-MM-DD HH:mm:ss"
          :default-time="rangePickerDefaultTime"
          :popper-class="emberRangePickerPopperClass"
          unlink-panels
          :icon="Calendar"
        />

        <template #actions>
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
        </template>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
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

      <template #pagination>
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
      </template>
    </EmberTableCard>
  </div>
</template>
