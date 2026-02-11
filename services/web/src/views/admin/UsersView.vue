<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getUsers, extendUserExpiry, toggleUserStatus, deleteUser, resetUserPassword } from '@/api/admin'

const tableData = ref([])
const total = ref(0)
const loading = ref(false)
const queryParams = ref({
  page: 1,
  pageSize: 10,
  search: ''
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getUsers(queryParams.value)
    tableData.value = res.data
    total.value = res.total
  } finally {
    loading.value = false
  }
}

const handleExtend = async (row: any) => {
  try {
    await ElMessageBox.prompt('请输入延长天数', '延长到期时间', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputPattern: /^\d+$/,
      inputErrorMessage: '请输入数字',
      inputValue: '30'
    }).then(async ({ value }) => {
      await extendUserExpiry(row.id, parseInt(value))
      ElMessage.success('延长成功')
      fetchData()
    })
  } catch {
    // cancelled
  }
}

const handleToggle = async (row: any) => {
  try {
    await toggleUserStatus(row.id)
    ElMessage.success(row.isActive ? '已禁用' : '已启用')
    fetchData()
  } catch {
    // error
  }
}

const handleDelete = async (row: any) => {
  try {
    await ElMessageBox.confirm('确定删除该用户吗？此操作不可恢复', '警告', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await deleteUser(row.id)
    ElMessage.success('删除成功')
    fetchData()
  } catch {
    // cancelled
  }
}

const handleResetPassword = async (row: any) => {
  try {
    const { value } = await ElMessageBox.prompt('请输入新密码 (留空生成随机密码)', '重置密码', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
    })
    
    let password = value
    if (!password) {
      password = Math.random().toString(36).slice(-8) + Math.random().toString(36).slice(-8)
      await ElMessageBox.alert(`已生成随机密码: ${password}`, '提示')
    }
    
    await resetUserPassword(row.id, password)
    ElMessage.success('密码重置成功')
    loadUsers()
  } catch {
    // cancelled
  }
}

// Format date helper
const formatDate = (dateStr: string) => {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString()
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>用户列表</span>
        <el-input
          v-model="queryParams.search"
          placeholder="搜索用户名/邮箱"
          style="width: 200px"
          @keyup.enter="fetchData"
        >
          <template #append>
            <el-button icon="Search" @click="fetchData" />
          </template>
        </el-input>
      </div>
    </template>
    
    <el-table :data="tableData" v-loading="loading" style="width: 100%">
      <el-table-column prop="username" label="用户名" />
      <el-table-column prop="email" label="邮箱" />
      <el-table-column prop="embyId" label="Emby ID" />
      <el-table-column label="状态">
        <template #default="{ row }">
          <el-tag :type="row.isActive ? 'success' : 'danger'">
            {{ row.isActive ? '正常' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="到期时间">
        <template #default="{ row }">
          {{ formatDate(row.expiresAt) }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="250">
        <template #default="{ row }">
          <el-button link type="primary" @click="handleExtend(row)">延长</el-button>
          <el-button link type="warning" @click="handleToggle(row)">
            {{ row.isActive ? '禁用' : '启用' }}
          </el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
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
