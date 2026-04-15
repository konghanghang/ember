<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Delete, Refresh, EditPen, Goods } from '@element-plus/icons-vue'
import { createPlan, deletePlan, getPlanGroups, getPlans, updatePlan } from '@/api/admin'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
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

const resetFilters = () => {
  queryParams.value.page = 1
  queryParams.value.showAll = true
  queryParams.value.planGroup = ''
  fetchData()
}

onMounted(async () => {
  await fetchPlanGroups()
  await fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      :title="props.embedded ? '方案池' : '付费方案管理'"
      :description="props.embedded ? '管理订阅购买套餐，并按套餐分组筛选当前结果。' : '管理订阅购买套餐，支持 USD、HKD、CNY'"
    >
      <template #titleSuffix>
        <span
          v-if="!props.embedded"
          class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500"
        >
          {{ activeCount }}/{{ total }} 启用
        </span>
      </template>

      <template #actions>
        <div class="flex flex-wrap items-center gap-3">
          <button
            @click="fetchData"
            class="inline-flex h-11 w-11 items-center justify-center cursor-pointer rounded-xl border border-gray-200 bg-white text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
            aria-label="刷新付费方案列表"
            title="刷新列表"
          >
            <el-icon :size="20"><Refresh /></el-icon>
          </button>
          <button
            @click="dialogVisible = true"
            class="btn-ember inline-flex cursor-pointer items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
          >
            <el-icon><Plus /></el-icon>
            <span>新建方案</span>
          </button>
        </div>
      </template>

      <div v-if="props.embedded" class="mt-4 flex flex-wrap items-center gap-2">
        <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-500">当前结果 {{ total }} 条</span>
        <span class="rounded-full bg-emerald-50 px-2.5 py-1 text-xs text-emerald-600">启用中 {{ activeCount }} 条</span>
      </div>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_auto]"
        content-class="grid grid-cols-1 gap-3 md:grid-cols-2"
        actions-class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0"
      >
        <EmberSelectField
          v-model="queryParams.planGroup"
          label="套餐分组"
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
        </EmberSelectField>

        <div class="space-y-1.5">
          <label class="text-xs font-semibold tracking-wide text-gray-500">显示范围</label>
          <div class="flex h-[42px] items-center justify-between rounded-xl border border-gray-200 bg-white px-3">
            <span class="text-sm text-gray-600">包含下架方案</span>
            <el-switch v-model="queryParams.showAll" size="small" @change="handleFilterChange" />
          </div>
        </div>

        <template #actions>
          <button
            @click="resetFilters"
            class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            重置
          </button>
        </template>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
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
      <template #pagination>
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
      </template>
    </EmberTableCard>

    <EmberFormDialog
      v-model="dialogVisible"
      title="新建付费方案"
      width="520px"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="方案名称">
            <el-input v-model="form.name" placeholder="例如：月度会员" class="input-ember" />
          </el-form-item>

          <el-form-item label="描述">
            <el-input v-model="form.description" type="textarea" :rows="3" placeholder="可选，展示在购买卡片" class="input-ember" />
          </el-form-item>

          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="时长（天）">
              <el-input-number v-model="form.days" :min="1" class="w-full !w-full form-number" />
            </el-form-item>

            <el-form-item label="价格">
              <el-input-number v-model="form.priceDisplay" :min="0.01" :step="0.01" :precision="2" class="w-full !w-full form-number" />
            </el-form-item>
          </div>

          <el-form-item label="币种">
            <el-select v-model="form.currency" class="w-full form-select" placeholder="选择币种">
              <el-option
                v-for="option in currencyOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="套餐组">
            <el-select v-model="form.planGroup" class="w-full form-select" placeholder="选择套餐组">
              <el-option
                v-for="option in planGroupOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="排序">
            <el-input-number v-model="form.sortOrder" :min="0" class="w-full !w-full form-number" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="px-6 pb-6 pt-0 flex justify-end gap-3">
          <button
            @click="dialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleCreate"
            :disabled="creating"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
          >
            {{ creating ? '创建中...' : '确认创建' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="editDialogVisible"
      title="编辑付费方案"
      width="520px"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="方案名称">
            <el-input v-model="editForm.name" placeholder="例如：月度会员" class="input-ember" />
          </el-form-item>

          <el-form-item label="描述">
            <el-input v-model="editForm.description" type="textarea" :rows="3" placeholder="可选，展示在购买卡片" class="input-ember" />
          </el-form-item>

          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="时长（天）">
              <el-input-number v-model="editForm.days" :min="1" class="w-full !w-full form-number" />
            </el-form-item>

            <el-form-item label="价格">
              <el-input-number v-model="editForm.priceDisplay" :min="0.01" :step="0.01" :precision="2" class="w-full !w-full form-number" />
            </el-form-item>
          </div>

          <el-form-item label="币种">
            <el-select v-model="editForm.currency" class="w-full form-select" placeholder="选择币种">
              <el-option
                v-for="option in currencyOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="套餐组">
            <el-select v-model="editForm.planGroup" class="w-full form-select" placeholder="选择套餐组">
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
              <el-input-number v-model="editForm.sortOrder" :min="0" class="w-full !w-full form-number" />
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
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleUpdate"
            :disabled="updating"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
          >
            {{ updating ? '保存中...' : '保存修改' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>

<style scoped>
:deep(.form-select .el-select__placeholder),
:deep(.form-select .el-select__selected-item) {
  font-size: 0.875rem;
}

:deep(.form-select .el-select__wrapper) {
  min-height: 42px;
  border-radius: 0.75rem;
  background-color: #f9fafb !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.form-select:hover .el-select__wrapper) {
  background-color: #ffffff !important;
}

:deep(.form-select .el-select__wrapper.is-focused),
:deep(.form-select .el-select__wrapper.is-focus),
:deep(.form-select.is-focus .el-select__wrapper) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}
</style>
