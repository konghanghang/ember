<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getRedemptionCodes, createRedemptionCode, deleteRedemptionCode } from '@/api/admin'
import type { CreateRedemptionCodeRequest, RedemptionCode } from '@/types/api'

const tableData = ref<RedemptionCode[]>([])
const total = ref(0)
const loading = ref(false)
const queryParams = ref({
  page: 1,
  pageSize: 10,
  showAll: false
})

const dialogVisible = ref(false)
const form = ref<CreateRedemptionCodeRequest>({
  maxUses: 1,
  defaultDays: 30,
  expiresAt: null
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getRedemptionCodes(queryParams.value)
    tableData.value = res.data
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  try {
    await createRedemptionCode(form.value)
    ElMessage.success('创建成功')
    dialogVisible.value = false
    await fetchData()
  } catch {
    // handled by interceptor
  }
}

const handleDelete = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定删除该兑换码吗？', '警告', { type: 'warning' })
    await deleteRedemptionCode(id)
    ElMessage.success('删除成功')
    await fetchData()
  } catch {
    // cancelled
  }
}

const formatDate = (dateStr?: string | null) => {
  if (!dateStr) return '永久有效'
  return new Date(dateStr).toLocaleString()
}

onMounted(fetchData)
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>兑换码列表</span>
        <div class="flex items-center gap-3">
          <el-switch v-model="queryParams.showAll" active-text="显示全部" @change="fetchData" />
          <el-button type="primary" @click="dialogVisible = true">生成兑换码</el-button>
        </div>
      </div>
    </template>

    <el-table :data="tableData" v-loading="loading" style="width: 100%">
      <el-table-column prop="code" label="兑换码" />
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
      <el-table-column label="操作" width="100">
        <template #default="{ row }">
          <el-button link type="danger" @click="handleDelete(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="生成兑换码">
      <el-form :model="form" label-width="100px">
        <el-form-item label="最大使用次数">
          <el-input-number v-model="form.maxUses" :min="1" />
        </el-form-item>
        <el-form-item label="有效天数">
          <el-input-number v-model="form.defaultDays" :min="1" />
        </el-form-item>
        <el-form-item label="过期时间">
          <el-date-picker
            v-model="form.expiresAt"
            type="datetime"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            placeholder="可选，不填则永久有效"
            clearable
          />
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
