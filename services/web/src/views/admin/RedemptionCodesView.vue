<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Calendar, Clock, Ticket, Plus, Delete, Refresh, Search } from '@element-plus/icons-vue'
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
const generating = ref(false)
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
  if (form.value.maxUses < 1 || form.value.defaultDays < 1) {
    ElMessage.warning('请输入有效的数值')
    return
  }
  
  generating.value = true
  try {
    await createRedemptionCode(form.value)
    ElMessage.success('兑换码生成成功')
    dialogVisible.value = false
    await fetchData()
  } catch {
    // handled
  } finally {
    generating.value = false
  }
}

const handleDelete = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定删除该兑换码吗？已使用该码的用户不受影响。', '删除确认', { 
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    })
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

const getUsageStatus = (row: RedemptionCode) => {
  if (row.usedCount >= row.maxUses) return { type: 'danger', text: '已耗尽' }
  if (row.expiresAt && new Date(row.expiresAt) < new Date()) return { type: 'info', text: '已过期' }
  return { type: 'success', text: '有效' }
}

onMounted(fetchData)
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          兑换码管理
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">{{ total }} 个兑换码</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">生成和管理注册/续期兑换码</p>
      </div>
      
      <div class="flex items-center gap-3">
        <div class="flex items-center gap-2 px-3 py-2 bg-gray-50 rounded-lg border border-gray-100">
          <span class="text-sm text-gray-600">显示全部</span>
          <el-switch v-model="queryParams.showAll" @change="fetchData" size="small" />
        </div>
        <button 
          @click="fetchData" 
          class="p-2 text-gray-500 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
          title="刷新列表"
        >
          <el-icon :size="20"><Refresh /></el-icon>
        </button>
        <button 
          @click="dialogVisible = true"
          class="flex items-center gap-2 px-4 py-2 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg active:scale-95"
        >
          <el-icon><Plus /></el-icon>
          <span>生成兑换码</span>
        </button>
      </div>
    </div>

    <!-- Table -->
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <el-table 
        :data="tableData" 
        v-loading="loading" 
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
        <el-table-column label="兑换码" min-width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded-lg bg-orange-50 flex items-center justify-center text-orange-500">
                <el-icon><Ticket /></el-icon>
              </div>
              <span class="font-mono font-medium text-gray-900 select-all">{{ row.code }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="getUsageStatus(row).type as any" effect="light" round size="small">
              {{ getUsageStatus(row).text }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="使用情况" min-width="150">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <el-progress 
                :percentage="Math.min((row.usedCount / row.maxUses) * 100, 100)" 
                :show-text="false"
                :status="row.usedCount >= row.maxUses ? 'success' : ''"
                class="w-24"
              />
              <span class="text-xs text-gray-500">{{ row.usedCount }} / {{ row.maxUses }}</span>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column label="有效期" min-width="120">
          <template #default="{ row }">
            <div class="flex items-center gap-1 text-gray-600">
              <el-icon><Clock /></el-icon>
              <span>{{ row.defaultDays }} 天</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="过期时间" min-width="180">
          <template #default="{ row }">
            <span :class="{'text-gray-400': !row.expiresAt}">{{ formatDate(row.expiresAt) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <button 
              @click="handleDelete(row.id)"
              class="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
              title="删除"
            >
              <el-icon :size="18"><Delete /></el-icon>
            </button>
          </template>
        </el-table-column>
      </el-table>

      <!-- Pagination -->
      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="fetchData"
          background
        />
      </div>
    </div>

    <!-- Create Dialog -->
    <el-dialog 
      v-model="dialogVisible" 
      title="生成兑换码" 
      width="480px"
      align-center
      class="rounded-2xl"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="最大使用次数">
              <el-input-number v-model="form.maxUses" :min="1" class="w-full !w-full" />
            </el-form-item>
            <el-form-item label="有效天数 (激活后)">
              <el-input-number v-model="form.defaultDays" :min="1" class="w-full !w-full" />
            </el-form-item>
          </div>

          <el-form-item label="兑换码过期时间 (可选)">
            <el-date-picker
              v-model="form.expiresAt"
              type="datetime"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              placeholder="不填则永久有效"
              :prefix-icon="Calendar"
              clearable
              class="w-full !w-full input-ember"
            />
            <div class="text-xs text-gray-400 mt-1">设置兑换码本身的有效期，过期后无法兑换。</div>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="px-6 pb-6 pt-0 flex justify-end gap-3">
          <button 
            @click="dialogVisible = false"
            class="px-4 py-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors font-medium"
          >
            取消
          </button>
          <button 
            @click="handleCreate" 
            :disabled="generating"
            class="px-6 py-2 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg disabled:opacity-70"
          >
            {{ generating ? '生成中...' : '确认生成' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
:deep(.el-table) {
  --el-table-header-bg-color: #f9fafb;
}

:deep(.el-dialog__header) {
  margin-right: 0;
  border-bottom: 1px solid #f3f4f6;
  padding: 20px 24px;
}

:deep(.el-dialog__body) {
  padding: 0;
}

:deep(.el-dialog__footer) {
  padding: 0;
}
</style>
