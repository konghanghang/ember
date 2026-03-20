<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Search, Ticket, UserFilled } from '@element-plus/icons-vue'
import { getAllRedemptions } from '@/api/admin'
import type { Redemption } from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

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
  const date = new Date(value)

  return {
    date: date.toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit'
    }),
    time: date.toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit'
    })
  }
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h1 v-if="!props.embedded" class="text-2xl font-bold text-gray-900 flex items-center gap-2">
            兑换历史
            <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Total: {{ total }}</span>
          </h1>
          <div class="flex flex-wrap items-center gap-2" :class="props.embedded ? '' : 'mt-2'">
            <span class="text-sm font-semibold text-gray-900">兑换记录</span>
            <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-500">当前结果 {{ total }} 条</span>
          </div>
          <p class="text-sm text-gray-500" :class="props.embedded ? 'mt-0.5' : 'mt-2'">查看所有用户的兑换码使用记录。</p>
        </div>

        <slot name="tabs" />
      </div>

      <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
        <div class="flex flex-col gap-3 xl:flex-row xl:items-end">
          <div class="flex flex-1 flex-wrap gap-3">
            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">用户名</label>
              <div class="relative w-full group xl:w-[260px]">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><UserFilled /></el-icon>
                </div>
                <input
                  v-model="queryParams.username"
                  type="text"
                  autocomplete="off"
                  aria-label="按用户名筛选"
                  placeholder="输入登录用户名筛选"
                  class="filter-input w-full pl-10 pr-4"
                  @keyup.enter="handleSearch"
                />
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">兑换码</label>
              <div class="relative w-full group xl:w-[320px]">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Ticket /></el-icon>
                </div>
                <input
                  v-model="queryParams.code"
                  type="text"
                  autocomplete="off"
                  aria-label="按兑换码筛选"
                  placeholder="输入兑换码筛选"
                  class="filter-input w-full pl-10 pr-4"
                  @keyup.enter="handleSearch"
                />
              </div>
            </div>
          </div>

          <div class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0">
            <button
              @click="handleReset"
              class="px-4 py-2.5 text-sm text-gray-700 bg-white border border-gray-200 hover:bg-gray-100 rounded-xl transition-colors cursor-pointer"
            >
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
            <el-tag type="success">{{ row.days }} 天</el-tag>
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
      </el-table>

      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
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
