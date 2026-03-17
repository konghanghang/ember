<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Search, Ticket, UserFilled } from '@element-plus/icons-vue'
import { getAllRedemptions } from '@/api/admin'
import { formatDate } from '@/utils/date'
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
  userId: '',
  code: ''
})

const fetchData = async () => {
  loading.value = true
  try {
    const params: { page: number; pageSize: number; userId?: string; code?: string } = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize
    }
    if (queryParams.value.userId) {
      params.userId = queryParams.value.userId
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
  queryParams.value.userId = ''
  queryParams.value.code = ''
  queryParams.value.page = 1
  fetchData()
}

const handleSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <div v-if="!props.embedded" class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          兑换历史
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Total: {{ total }}</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">查看所有用户的兑换码使用记录</p>
      </div>

      <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
        <div class="flex flex-col lg:flex-row lg:items-end gap-3">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3 w-full">
            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">用户 ID</label>
              <div class="relative w-full group">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><UserFilled /></el-icon>
                </div>
                <input
                  v-model="queryParams.userId"
                  type="text"
                  autocomplete="off"
                  aria-label="按用户 ID 筛选"
                  placeholder="输入用户 ID 筛选"
                  class="filter-input w-full pl-10 pr-4"
                  @keyup.enter="handleSearch"
                />
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">兑换码</label>
              <div class="relative w-full group">
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

          <div class="flex items-center gap-2 self-end lg:ml-auto lg:shrink-0">
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
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="code" label="兑换码" width="180" />
        <el-table-column prop="days" label="延长天数" width="120">
          <template #default="{ row }">
            <el-tag type="success">{{ row.days }} 天</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="兑换时间" width="200">
          <template #default="{ row }">
            {{ formatDate(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column prop="userId" label="用户 ID" min-width="200">
          <template #default="{ row }">
            <code class="text-xs text-gray-600">{{ row.userId }}</code>
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
