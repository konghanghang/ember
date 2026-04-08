<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Delete, Refresh, EditPen, Goods } from '@element-plus/icons-vue'
import { createPlan, deletePlan, getPlanGroups, getPlans, updatePlan } from '@/api/admin'
import type { CreatePlanRequest, ManagedPlanGroup, Plan, PlanGroup, UpdatePlanRequest } from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

const planGroups = ref<ManagedPlanGroup[]>([])

const tableData = ref<Plan[]>([])
const total = ref(0)
const loading = ref(false)
const creating = ref(false)
const updating = ref(false)

const queryParams = ref({
  page: 1,
  pageSize: 10,
  showAll: true,
  planGroup: '' as PlanGroup | ''
})

const dialogVisible = ref(false)
const editDialogVisible = ref(false)

const form = ref({
  name: '',
  description: '',
  days: 30,
  priceDisplay: 9.99,
  currency: 'usd',
  planGroup: '' as PlanGroup,
  sortOrder: 0
})

const editForm = ref({
  id: '',
  name: '',
  description: '',
  days: 30,
  priceDisplay: 9.99,
  currency: 'usd',
  planGroup: '' as PlanGroup,
  isActive: true,
  sortOrder: 0
})

const currencyOptions = [
  { label: 'USD', value: 'usd' },
  { label: 'HKD', value: 'hkd' },
  { label: 'CNY', value: 'cny' }
]

const planGroupOptions = computed(() => planGroups.value.map(group => ({
  label: `${group.name} (${group.key})`,
  value: group.key
})))

const defaultPlanGroup = computed(() => planGroups.value.find(group => group.isDefault) ?? null)
const activeCount = computed(() => tableData.value.filter(item => item.isActive).length)

const handlePageSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const applyDefaultPlanGroupToForms = () => {
  const fallbackGroup = defaultPlanGroup.value?.key || planGroups.value[0]?.key || ''
  if (!form.value.planGroup) {
    form.value.planGroup = fallbackGroup
  }
  if (!editForm.value.planGroup) {
    editForm.value.planGroup = fallbackGroup
  }
}

const fetchPlanGroups = async () => {
  const res = await getPlanGroups()
  planGroups.value = res.data ?? []
  applyDefaultPlanGroupToForms()
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getPlans(queryParams.value)
    tableData.value = res.data || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

const formatPrice = (price: number, currency: string = 'usd') => {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: currency.toUpperCase(),
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(price / 100)
}

const resetCreateForm = () => {
  form.value = {
    name: '',
    description: '',
    days: 30,
    priceDisplay: 9.99,
    currency: 'usd',
    planGroup: defaultPlanGroup.value?.key || planGroups.value[0]?.key || '',
    sortOrder: 0
  }
}

const validatePriceDisplay = (value: number) => {
  return Number.isFinite(value) && value > 0
}

const handleCreate = async () => {
  if (!form.value.name.trim()) {
    ElMessage.warning('请输入方案名称')
    return
  }
  if (form.value.days < 1) {
    ElMessage.warning('天数必须大于 0')
    return
  }
  if (!validatePriceDisplay(form.value.priceDisplay)) {
    ElMessage.warning('请输入有效价格')
    return
  }
  if (!form.value.planGroup) {
    ElMessage.warning('请选择套餐分组')
    return
  }

  const payload: CreatePlanRequest = {
    name: form.value.name.trim(),
    description: form.value.description.trim(),
    days: form.value.days,
    price: Math.round(form.value.priceDisplay * 100),
    currency: form.value.currency,
    planGroup: form.value.planGroup,
    sortOrder: form.value.sortOrder
  }

  creating.value = true
  try {
    await createPlan(payload)
    ElMessage.success('方案创建成功')
    dialogVisible.value = false
    resetCreateForm()
    await fetchData()
  } finally {
    creating.value = false
  }
}

const openEditDialog = (row: Plan) => {
  editForm.value = {
    id: row.id,
    name: row.name,
    description: row.description || '',
    days: row.days,
    priceDisplay: row.price / 100,
    currency: row.currency || 'usd',
    planGroup: row.planGroup || defaultPlanGroup.value?.key || '',
    isActive: row.isActive,
    sortOrder: row.sortOrder
  }
  editDialogVisible.value = true
}

const handleUpdate = async () => {
  if (!editForm.value.name.trim()) {
    ElMessage.warning('请输入方案名称')
    return
  }
  if (editForm.value.days < 1) {
    ElMessage.warning('天数必须大于 0')
    return
  }
  if (!validatePriceDisplay(editForm.value.priceDisplay)) {
    ElMessage.warning('请输入有效价格')
    return
  }
  if (!editForm.value.planGroup) {
    ElMessage.warning('请选择套餐分组')
    return
  }

  const payload: UpdatePlanRequest = {
    name: editForm.value.name.trim(),
    description: editForm.value.description.trim(),
    days: editForm.value.days,
    price: Math.round(editForm.value.priceDisplay * 100),
    currency: editForm.value.currency,
    planGroup: editForm.value.planGroup,
    isActive: editForm.value.isActive,
    sortOrder: editForm.value.sortOrder
  }

  updating.value = true
  try {
    await updatePlan(editForm.value.id, payload)
    ElMessage.success('方案更新成功')
    editDialogVisible.value = false
    await fetchData()
  } finally {
    updating.value = false
  }
}

const handleDelete = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定下架该方案吗？下架后用户无法新购，历史支付记录保留。', '下架确认', {
      confirmButtonText: '下架',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    })

    await deletePlan(id)
    ElMessage.success('已下架')
    await fetchData()
  } catch {
    // cancelled
  }
}

const handleFilterChange = () => {
  queryParams.value.page = 1
  fetchData()
}

onMounted(async () => {
  await fetchPlanGroups()
  await fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <div v-if="!props.embedded" class="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          付费方案管理
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">{{ activeCount }}/{{ total }} 启用</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">管理订阅购买套餐，支持 USD、HKD、CNY</p>
      </div>

      <div class="flex items-center gap-3">
        <div class="flex items-center gap-2 px-3 py-2 bg-gray-50 rounded-lg border border-gray-100">
          <span class="text-sm text-gray-600">显示全部</span>
          <el-switch v-model="queryParams.showAll" @change="handleFilterChange" size="small" />
        </div>
        <el-select
          v-model="queryParams.planGroup"
          class="!w-[180px]"
          placeholder="全部分组"
          @change="handleFilterChange"
        >
          <el-option label="全部分组" value="" />
          <el-option
            v-for="option in planGroupOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </el-select>
        <button
          @click="fetchData"
          class="cursor-pointer rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
          aria-label="刷新付费方案列表"
          title="刷新列表"
        >
          <el-icon :size="20"><Refresh /></el-icon>
        </button>
        <button
          @click="dialogVisible = true"
          class="flex items-center gap-2 px-4 py-2 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg active:scale-95"
        >
          <el-icon><Plus /></el-icon>
          <span>新建方案</span>
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
        <el-table-column label="方案" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded-lg bg-orange-50 flex items-center justify-center text-orange-500">
                <el-icon><Goods /></el-icon>
              </div>
              <div>
                <div class="font-semibold text-gray-900">{{ row.name }}</div>
                <div class="text-xs text-gray-500">{{ row.description || '无描述' }}</div>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="时长" width="100">
          <template #default="{ row }">
            <span class="font-medium text-gray-700">{{ row.days }} 天</span>
          </template>
        </el-table-column>

        <el-table-column label="价格" width="120">
          <template #default="{ row }">
            <span class="font-semibold text-gray-900">{{ formatPrice(row.price, row.currency) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="币种" width="100">
          <template #default="{ row }">
            <span class="text-gray-600 uppercase">{{ row.currency }}</span>
          </template>
        </el-table-column>

        <el-table-column label="分组" min-width="140">
          <template #default="{ row }">
            <el-tag effect="light" round size="small" :type="row.planGroup === defaultPlanGroup?.key ? 'warning' : 'success'">
              {{ row.planGroupName || row.planGroup }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.isActive ? 'success' : 'info'" effect="light" round size="small">
              {{ row.isActive ? '启用' : '下架' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="排序" width="100">
          <template #default="{ row }">
            <span class="text-gray-600">{{ row.sortOrder }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <button
              @click="openEditDialog(row)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
              aria-label="编辑付费方案"
              title="编辑"
            >
              <el-icon :size="18"><EditPen /></el-icon>
            </button>
            <button
              @click="handleDelete(row.id)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
              aria-label="下架付费方案"
              title="下架"
            >
              <el-icon :size="18"><Delete /></el-icon>
            </button>
          </template>
        </el-table-column>
      </el-table>

      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="fetchData"
          @size-change="handlePageSizeChange"
          background
        />
      </div>
    </div>

    <el-dialog v-model="dialogVisible" title="新建付费方案" width="520px" align-center append-to-body>
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="方案名称">
            <el-input v-model="form.name" placeholder="例如：月度会员" />
          </el-form-item>

          <el-form-item label="描述">
            <el-input v-model="form.description" type="textarea" :rows="3" placeholder="可选，展示在购买卡片" />
          </el-form-item>

          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="时长（天）">
              <el-input-number v-model="form.days" :min="1" class="w-full !w-full" />
            </el-form-item>

            <el-form-item label="价格">
              <el-input-number v-model="form.priceDisplay" :min="0.01" :step="0.01" :precision="2" class="w-full !w-full" />
            </el-form-item>
          </div>

          <el-form-item label="币种">
            <el-select v-model="form.currency" class="w-full" placeholder="选择币种">
              <el-option
                v-for="option in currencyOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="套餐组">
            <el-select v-model="form.planGroup" class="w-full" placeholder="选择套餐组">
              <el-option
                v-for="option in planGroupOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="排序">
            <el-input-number v-model="form.sortOrder" :min="0" class="w-full !w-full" />
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
            :disabled="creating"
            class="px-6 py-2 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg disabled:opacity-70"
          >
            {{ creating ? '创建中...' : '确认创建' }}
          </button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" title="编辑付费方案" width="520px" align-center append-to-body>
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="方案名称">
            <el-input v-model="editForm.name" placeholder="例如：月度会员" />
          </el-form-item>

          <el-form-item label="描述">
            <el-input v-model="editForm.description" type="textarea" :rows="3" placeholder="可选，展示在购买卡片" />
          </el-form-item>

          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="时长（天）">
              <el-input-number v-model="editForm.days" :min="1" class="w-full !w-full" />
            </el-form-item>

            <el-form-item label="价格">
              <el-input-number v-model="editForm.priceDisplay" :min="0.01" :step="0.01" :precision="2" class="w-full !w-full" />
            </el-form-item>
          </div>

          <el-form-item label="币种">
            <el-select v-model="editForm.currency" class="w-full" placeholder="选择币种">
              <el-option
                v-for="option in currencyOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="套餐组">
            <el-select v-model="editForm.planGroup" class="w-full" placeholder="选择套餐组">
              <el-option
                v-for="option in planGroupOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="排序">
              <el-input-number v-model="editForm.sortOrder" :min="0" class="w-full !w-full" />
            </el-form-item>

            <el-form-item label="状态">
              <div class="h-8 flex items-center">
                <el-switch v-model="editForm.isActive" active-text="启用" inactive-text="下架" />
              </div>
            </el-form-item>
          </div>
        </el-form>
      </div>
      <template #footer>
        <div class="px-6 pb-6 pt-0 flex justify-end gap-3">
          <button
            @click="editDialogVisible = false"
            class="px-4 py-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors font-medium"
          >
            取消
          </button>
          <button
            @click="handleUpdate"
            :disabled="updating"
            class="px-6 py-2 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg disabled:opacity-70"
          >
            {{ updating ? '保存中...' : '保存修改' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>
