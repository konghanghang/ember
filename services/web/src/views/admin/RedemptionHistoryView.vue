<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { getAllRedemptions } from '@/api/admin'
import { formatDate } from '@/utils/date'
import type { Redemption } from '@/types/api'

const tableData = ref<Redemption[]>([])
const loading = ref(false)
const total = ref(0)
const queryParams = ref({
  page: 1,
  pageSize: 10,
  userId: ''
})

const fetchData = async () => {
  loading.value = true
  try {
    const params: { page: number; pageSize: number; userId?: string } = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize
    }
    if (queryParams.value.userId) {
      params.userId = queryParams.value.userId
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
  <div class="p-6 space-y-6">
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900">兑换历史</h1>
          <p class="text-sm text-gray-500 mt-1">查看所有用户的兑换码使用记录</p>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-sm text-gray-500">
            共 <span class="font-semibold text-gray-900">{{ total }}</span> 条记录
          </span>
          <el-button :icon="Refresh" @click="fetchData" :loading="loading">
            刷新
          </el-button>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <el-form :inline="true">
        <el-form-item label="用户 ID">
          <el-input
            v-model="queryParams.userId"
            placeholder="输入用户 ID 筛选"
            clearable
            style="width: 200px"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">搜索</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ backgroundColor: '#f9fafb' }"
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

      <div class="mt-4 flex justify-end bg-gray-50/50 p-4 rounded-lg">
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="fetchData"
          @size-change="handleSizeChange"
        />
      </div>
    </div>
  </div>
</template>
