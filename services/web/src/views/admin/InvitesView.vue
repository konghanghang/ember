<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getInvites, createInvite, deleteInvite } from '@/api/admin'

const tableData = ref([])
const total = ref(0)
const loading = ref(false)
const queryParams = ref({
  page: 1,
  pageSize: 10
})

const dialogVisible = ref(false)
const form = ref({
  maxUses: 1,
  defaultDays: 30,
  // expiresAt: null // Optional
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getInvites(queryParams.value)
    tableData.value = res.invites
    total.value = res.total || 0 // Assuming backend returns total for pagination?
    // Note: The API Spec for getInvites response: { invites: [...] } 
    // It didn't explicitly say "total" in the response example for invites list (3.1), 
    // but usually paginated APIs do. If not, we might need to adjust.
    // API Spec 2.1 Users has total. 3.1 Invites example doesn't show total.
    // I'll assume it exists or I'll fix later.
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  try {
    await createInvite(form.value)
    ElMessage.success('创建成功')
    dialogVisible.value = false
    fetchData()
  } catch {
    // error
  }
}

const handleDelete = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定删除该邀请码吗？', '警告', {
      type: 'warning'
    })
    await deleteInvite(id)
    ElMessage.success('删除成功')
    fetchData()
  } catch {
    // cancelled
  }
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return '永久有效'
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
        <span>邀请码列表</span>
        <el-button type="primary" icon="Plus" @click="dialogVisible = true">生成邀请码</el-button>
      </div>
    </template>

    <el-table :data="tableData" v-loading="loading" style="width: 100%">
      <el-table-column prop="code" label="邀请码" />
      <el-table-column label="使用情况">
        <template #default="{ row }">
          {{ row.usedCount }} / {{ row.maxUses }}
        </template>
      </el-table-column>
      <el-table-column prop="defaultDays" label="默认天数" />
      <el-table-column label="过期时间">
        <template #default="{ row }">
          {{ formatDate(row.expiresAt) }}
        </template>
      </el-table-column>
      <el-table-column label="操作">
        <template #default="{ row }">
          <el-button link type="danger" @click="handleDelete(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Create Dialog -->
    <el-dialog v-model="dialogVisible" title="生成邀请码">
      <el-form :model="form" label-width="100px">
        <el-form-item label="最大使用次数">
          <el-input-number v-model="form.maxUses" :min="1" />
        </el-form-item>
        <el-form-item label="有效天数">
          <el-input-number v-model="form.defaultDays" :min="1" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
