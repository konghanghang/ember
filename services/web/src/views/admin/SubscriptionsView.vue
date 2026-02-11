<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getAllSubscriptions, approveSubscription, rejectSubscription } from '@/api/admin'

const tableData = ref([])
const total = ref(0)
const loading = ref(false)
const queryParams = ref({
  page: 1,
  pageSize: 10,
  status: 'PENDING'
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getAllSubscriptions(queryParams.value)
    tableData.value = res.data
    total.value = res.total
  } finally {
    loading.value = false
  }
}

const handleApprove = async (id: string) => {
  try {
    await approveSubscription(id)
    ElMessage.success('已批准')
    fetchData()
  } catch {
    // error
  }
}

const handleReject = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定拒绝该订阅申请吗？', '提示', {
      type: 'warning'
    })
    await rejectSubscription(id)
    ElMessage.success('已拒绝')
    fetchData()
  } catch {
    // cancelled
  }
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>订阅管理</span>
        <el-radio-group v-model="queryParams.status" @change="fetchData">
          <el-radio-button label="PENDING">待审核</el-radio-button>
          <el-radio-button label="APPROVED">已批准</el-radio-button>
          <el-radio-button label="REJECTED">已拒绝</el-radio-button>
        </el-radio-group>
      </div>
    </template>

    <el-table :data="tableData" v-loading="loading" style="width: 100%">
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="type" label="类型" width="80">
        <template #default="{ row }">
          <el-tag>{{ row.type === 'MOVIE' ? '电影' : '剧集' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="user.username" label="申请用户" />
      <el-table-column prop="tmdbId" label="TMDB ID" />
      <el-table-column prop="note" label="备注" />
      <el-table-column label="提交时间">
        <template #default="{ row }">
          {{ new Date(row.createdAt).toLocaleString() }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" v-if="queryParams.status === 'PENDING'">
        <template #default="{ row }">
          <el-button link type="success" @click="handleApprove(row.id)">批准</el-button>
          <el-button link type="danger" @click="handleReject(row.id)">拒绝</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pagination">
      <el-pagination
        v-model:current-page="queryParams.page"
        v-model:page-size="queryParams.pageSize"
        :total="total"
        layout="total, prev, pager, next"
        @current-change="fetchData"
      />
    </div>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
