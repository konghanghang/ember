<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Search, Ticket, UserFilled, RefreshRight, Clock } from '@element-plus/icons-vue'
import { getAllRedemptions } from '@/api/admin'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { formatDateOnly, formatTimeOnly } from '@/utils/date'
import type { Redemption } from '@/types/api'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const tableData = ref<Redemption[]>([])
const loading = ref(false)
const total = ref(0)
const queryParams = ref({
  page: 1,
  pageSize: 10,
  username: '',
  code: ''
})

const fetchData = async () => {
  loading.value = true
  try {
    const params: { page: number; pageSize: number; username?: string; code?: string } = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize
    }
    if (queryParams.value.username) {
      params.username = queryParams.value.username.trim()
    }
    if (queryParams.value.code) {
      params.code = queryParams.value.code.trim()
    }

    const res = await getAllRedemptions(params)
    tableData.value = res.data
    total.value = res.total
  } catch {
    // handled
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleReset = () => {
  queryParams.value.username = ''
  queryParams.value.code = ''
  queryParams.value.page = 1
  fetchData()
}

const handleSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const formatRedeemedAt = (value: string) => {
  return {
    date: formatDateOnly(value),
    time: formatTimeOnly(value)
  }
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard :hide-title="props.embedded" title="兑换记录">
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">
          当前结果 {{ total }} 条
        </span>
      </template>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_auto]"
        content-class="grid grid-cols-1 gap-3 lg:grid-cols-[260px_320px]"
        actions-class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0"
      >
        <EmberSearchInput
          v-model="queryParams.username"
          label="用户名"
          type="text"
          inputmode="text"
          autocomplete="off"
          aria-label="按用户名筛选"
          placeholder="输入登录用户名筛选"
          :icon="UserFilled"
          @enter="handleSearch"
        />

        <EmberSearchInput
          v-model="queryParams.code"
          label="兑换码"
          type="text"
          inputmode="text"
          autocomplete="off"
          aria-label="按兑换码筛选"
          placeholder="输入兑换码筛选"
          :icon="Ticket"
          @enter="handleSearch"
        />

        <template #actions>
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
        </template>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
        <el-table-column label="用户名" min-width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
                <el-icon><UserFilled /></el-icon>
              </div>
              <span class="font-medium text-gray-900">{{ row.username || '未知用户' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="兑换码" min-width="200">
          <template #default="{ row }">
            <code class="inline-flex rounded-lg border border-amber-100 bg-amber-50 px-3 py-1.5 font-mono text-sm font-medium text-amber-700">
              {{ row.code }}
            </code>
          </template>
        </el-table-column>
        <el-table-column prop="days" label="延长天数" width="120">
          <template #default="{ row }">
            <span class="inline-flex items-center gap-1 text-gray-700">
              <el-icon class="text-emerald-600"><Clock /></el-icon>
              <span>+{{ row.days }} 天</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="兑换时间" width="200">
          <template #default="{ row }">
            <div class="flex flex-col leading-tight">
              <span class="text-sm font-medium text-gray-900">{{ formatRedeemedAt(row.createdAt).date }}</span>
              <span class="mt-1 text-xs text-gray-500">{{ formatRedeemedAt(row.createdAt).time }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="userId" label="用户 ID" min-width="200">
          <template #default="{ row }">
            <code class="inline-flex rounded-md bg-gray-100 px-2.5 py-1 text-xs text-gray-600">{{ row.userId }}</code>
          </template>
        </el-table-column>

      <template #pagination>
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="fetchData"
          @size-change="handleSizeChange"
          background
        />
      </template>
    </EmberTableCard>
  </div>
</template>
