<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import {
  getTVCalendar,
  getTVCalendarSubscriptions,
  subscribeTVCalendar,
  unsubscribeTVCalendar
} from '@/api/console'
import { refreshTVCalendar } from '@/api/admin'
import type { TVCalendarItem, TVCalendarStatus, TVCalendarSubscription } from '@/types/api'

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)

const loading = ref(false)
const refreshing = ref(false)
const subscribeLoading = ref(false)
const subscribeDialogVisible = ref(false)

const calendarItems = ref<TVCalendarItem[]>([])
const subscriptions = ref<TVCalendarSubscription[]>([])

const filters = reactive({
  startDate: defaultStartDate(),
  endDate: defaultEndDate(),
  status: '' as TVCalendarStatus | ''
})

const subscribeForm = reactive({
  tmdbId: '',
  showName: '',
  posterUrl: ''
})

const statusOptions: Array<{ label: string; value: TVCalendarStatus | '' }> = [
  { label: '全部状态', value: '' },
  { label: '已入库', value: 'ready' },
  { label: '缺失', value: 'missing' },
  { label: '待播', value: 'upcoming' },
  { label: '今日播出', value: 'today' }
]

function defaultStartDate(): string {
  const d = new Date()
  d.setDate(d.getDate() - 7)
  return formatDate(d)
}

function defaultEndDate(): string {
  const d = new Date()
  d.setDate(d.getDate() + 30)
  return formatDate(d)
}

function formatDate(date: Date): string {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

function formatAirDate(value: string): string {
  if (!value) {
    return '-'
  }
  return value.slice(0, 10)
}

function formatEpisodeCode(season: number, episode: number): string {
  return `S${String(season).padStart(2, '0')}E${String(episode).padStart(2, '0')}`
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

async function fetchSubscriptions(): Promise<void> {
  const res = await getTVCalendarSubscriptions()
  subscriptions.value = res.data || []
}

async function fetchCalendar(): Promise<void> {
  loading.value = true
  try {
    const params: { startDate: string; endDate: string; status?: TVCalendarStatus } = {
      startDate: filters.startDate,
      endDate: filters.endDate
    }
    if (filters.status) {
      params.status = filters.status
    }
    const res = await getTVCalendar(params)
    calendarItems.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function loadAll(): Promise<void> {
  await Promise.all([fetchSubscriptions(), fetchCalendar()])
}

function openSubscribeDialog(): void {
  subscribeForm.tmdbId = ''
  subscribeForm.showName = ''
  subscribeForm.posterUrl = ''
  subscribeDialogVisible.value = true
}

async function handleSubscribe(): Promise<void> {
  if (!subscribeForm.tmdbId.trim() || !subscribeForm.showName.trim()) {
    ElMessage.warning('tmdbId 和剧名不能为空')
    return
  }

  subscribeLoading.value = true
  try {
    await subscribeTVCalendar({
      tmdbId: subscribeForm.tmdbId.trim(),
      showName: subscribeForm.showName.trim(),
      posterUrl: subscribeForm.posterUrl.trim() || undefined
    })
    subscribeDialogVisible.value = false
    ElMessage.success('订阅成功')
    await loadAll()
  } finally {
    subscribeLoading.value = false
  }
}

async function handleUnsubscribe(subscription: TVCalendarSubscription): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确认取消订阅《${subscription.showName}》吗？`,
      '取消订阅',
      {
        type: 'warning',
        confirmButtonText: '确认',
        cancelButtonText: '取消'
      }
    )
  } catch {
    return
  }

  await unsubscribeTVCalendar(subscription.tmdbId)
  ElMessage.success('已取消订阅')
  await loadAll()
}

async function handleRefresh(): Promise<void> {
  refreshing.value = true
  try {
    if (isAdmin.value) {
      const res = await refreshTVCalendar({ force: false })
      ElMessage.success(`刷新完成，处理 ${res.count} 条`) 
    }
    await loadAll()
  } finally {
    refreshing.value = false
  }
}

function resetFilters(): void {
  filters.startDate = defaultStartDate()
  filters.endDate = defaultEndDate()
  filters.status = ''
  void fetchCalendar()
}

onMounted(() => {
  void loadAll()
})
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900">追剧日历</h1>
          <p class="text-sm text-gray-500 mt-1">同步 TMDB 剧集排期并跟踪入库状态</p>
        </div>

        <div class="flex flex-wrap gap-2">
          <el-button type="primary" plain :icon="Plus" @click="openSubscribeDialog">订阅剧集</el-button>
          <el-button :icon="Refresh" :loading="refreshing" @click="handleRefresh">刷新</el-button>
        </div>
      </div>

      <div class="mt-5 grid grid-cols-1 gap-3 lg:grid-cols-5">
        <el-date-picker
          v-model="filters.startDate"
          type="date"
          value-format="YYYY-MM-DD"
          format="YYYY-MM-DD"
          placeholder="开始日期"
        />
        <el-date-picker
          v-model="filters.endDate"
          type="date"
          value-format="YYYY-MM-DD"
          format="YYYY-MM-DD"
          placeholder="结束日期"
        />
        <el-select v-model="filters.status" placeholder="状态筛选" clearable>
          <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <el-button type="primary" :icon="Search" @click="fetchCalendar">查询</el-button>
        <el-button @click="resetFilters">重置</el-button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-lg font-semibold text-gray-900">我的订阅</h2>
        <span class="text-xs text-gray-500">{{ subscriptions.length }} 部</span>
      </div>

      <div v-if="subscriptions.length === 0" class="text-sm text-gray-400 py-2">暂无订阅剧集</div>
      <div v-else class="flex flex-wrap gap-2">
        <el-tag
          v-for="subscription in subscriptions"
          :key="subscription.id"
          type="info"
          effect="plain"
          class="!px-3 !py-2"
        >
          <span class="mr-2">{{ subscription.showName }}</span>
          <el-button link type="danger" :icon="Delete" @click.stop="handleUnsubscribe(subscription)">取消</el-button>
        </el-tag>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
      <el-table v-loading="loading" :data="calendarItems" stripe>
        <el-table-column prop="airDate" label="播出日期" width="120">
          <template #default="scope">{{ formatAirDate(scope.row.airDate) }}</template>
        </el-table-column>
        <el-table-column prop="showName" label="剧集" min-width="180" />
        <el-table-column label="集数" width="110">
          <template #default="scope">{{ formatEpisodeCode(scope.row.season, scope.row.episode) }}</template>
        </el-table-column>
        <el-table-column prop="episodeName" label="标题" min-width="200" />
        <el-table-column label="状态" width="120">
          <template #default="scope">
            <el-tag :type="statusTagType(scope.row.status)">{{ statusText(scope.row.status) }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="subscribeDialogVisible" title="订阅剧集" width="520px">
      <el-form label-position="top">
        <el-form-item label="TMDB ID">
          <el-input v-model="subscribeForm.tmdbId" placeholder="例如：1399" />
        </el-form-item>
        <el-form-item label="剧名">
          <el-input v-model="subscribeForm.showName" placeholder="例如：Game of Thrones" />
        </el-form-item>
        <el-form-item label="海报 URL（可选）">
          <el-input v-model="subscribeForm.posterUrl" placeholder="https://..." />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="subscribeDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="subscribeLoading" @click="handleSubscribe">确认订阅</el-button>
      </template>
    </el-dialog>
  </div>
</template>
