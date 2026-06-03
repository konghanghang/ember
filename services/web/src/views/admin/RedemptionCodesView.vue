<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Calendar, Clock, Ticket, Plus, Delete, Refresh, EditPen, CopyDocument, Search, CollectionTag } from '@element-plus/icons-vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { formatDateTime } from '@/utils/date'
import { getRedemptionCodes, createRedemptionCode, createRedemptionCodesBatch, updateRedemptionCode, deleteRedemptionCode, getPlanGroups } from '@/api/admin'
import type {
  CreateRedemptionCodeRequest,
  ManagedPlanGroup,
  PlanGroup,
  RedemptionCode,
  RedemptionCodeListQuery,
  RedemptionCodeStatusFilter,
  UpdateRedemptionCodeRequest
} from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

const maxBatchCreateCount = 100

const tableData = ref<RedemptionCode[]>([])
const total = ref(0)
const loading = ref(false)
const queryParams = ref<RedemptionCodeListQuery>({
  page: 1,
  pageSize: 10,
  showAll: false,
  code: '',
  status: '',
  registrationPlanGroup: ''
})
const planGroups = ref<ManagedPlanGroup[]>([])

const dialogVisible = ref(false)
const generating = ref(false)
const batchResultDialogVisible = ref(false)
const editDialogVisible = ref(false)
const editing = ref(false)
const form = ref<CreateRedemptionCodeRequest & { count: number }>({
  count: 1,
  maxUses: 1,
  defaultDays: 30,
  registrationPlanGroup: '',
  expiresAt: null,
  notes: ''
})
const batchCreatedCodes = ref<RedemptionCode[]>([])
const editForm = ref({
  id: '',
  usedCount: 0,
  maxUses: 1,
  defaultDays: 30,
  registrationPlanGroup: '' as PlanGroup | '',
  neverExpire: false,
  expiresAt: null as Date | null,
  notes: ''
})

const statusOptions: Array<{ label: string; value: RedemptionCodeStatusFilter }> = [
  { label: '有效', value: 'active' },
  { label: '已过期', value: 'expired' },
  { label: '已耗尽', value: 'exhausted' }
]

const planGroupOptions = computed(() => planGroups.value.map((group) => ({
  label: `${group.name}${group.isDefault ? '（默认）' : ''} (${group.key})`,
  value: group.key
})))

const defaultPlanGroup = computed(() => planGroups.value.find((group) => group.isDefault) ?? null)
const planGroupNameMap = computed(() => new Map(planGroups.value.map((group) => [group.key, group.name])))
const defaultPlanGroupKey = computed(() => defaultPlanGroup.value?.key ?? planGroups.value[0]?.key ?? '')

const handlePageSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const fetchData = async () => {
  loading.value = true
  try {
    const params: RedemptionCodeListQuery = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize,
      showAll: queryParams.value.showAll
    }

    if (queryParams.value.code?.trim()) {
      params.code = queryParams.value.code.trim()
    }
    if (queryParams.value.status) {
      params.status = queryParams.value.status
    }
    if (queryParams.value.registrationPlanGroup?.trim()) {
      params.registrationPlanGroup = queryParams.value.registrationPlanGroup.trim()
    }

    const res = await getRedemptionCodes(params)
    tableData.value = res.data
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

const fetchPlanGroups = async () => {
  try {
    const res = await getPlanGroups()
    planGroups.value = res.data ?? []
  } catch {
    planGroups.value = []
  }
}

const resetCreateForm = () => {
  form.value = {
    count: 1,
    maxUses: 1,
    defaultDays: 30,
    registrationPlanGroup: defaultPlanGroupKey.value,
    expiresAt: null,
    notes: ''
  }
}

const openCreateDialog = () => {
  resetCreateForm()
  dialogVisible.value = true
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleReset = () => {
  queryParams.value.code = ''
  queryParams.value.status = ''
  queryParams.value.registrationPlanGroup = ''
  queryParams.value.showAll = false
  queryParams.value.page = 1
  fetchData()
}

const handleCreate = async () => {
  if (form.value.count < 1 || form.value.count > maxBatchCreateCount) {
    ElMessage.warning(`批量数量必须在 1 到 ${maxBatchCreateCount} 之间`)
    return
  }
  if (form.value.maxUses < 1 || form.value.defaultDays < 1) {
    ElMessage.warning('请输入有效的数值')
    return
  }
  if (!form.value.registrationPlanGroup) {
    ElMessage.warning('请选择注册套餐分组')
    return
  }
  const trimmedNotes = form.value.notes?.trim() ?? ''
  if (trimmedNotes.length > 500) {
    ElMessage.warning('备注最多 500 字')
    return
  }

  const payload: CreateRedemptionCodeRequest = {
    maxUses: form.value.maxUses,
    defaultDays: form.value.defaultDays,
    registrationPlanGroup: form.value.registrationPlanGroup,
    expiresAt: form.value.expiresAt,
    notes: trimmedNotes || undefined
  }

  generating.value = true
  try {
    if (form.value.count === 1) {
      await createRedemptionCode(payload)
      ElMessage.success('兑换码生成成功')
    } else {
      const res = await createRedemptionCodesBatch({
        count: form.value.count,
        ...payload
      })
      batchCreatedCodes.value = res.data
      batchResultDialogVisible.value = true
      ElMessage.success(`已生成 ${res.count} 个兑换码`)
    }
    dialogVisible.value = false
    resetCreateForm()
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

const openEditDialog = (row: RedemptionCode) => {
  editForm.value = {
    id: row.id,
    usedCount: row.usedCount,
    maxUses: row.maxUses,
    defaultDays: row.defaultDays,
    registrationPlanGroup: row.registrationPlanGroup || defaultPlanGroupKey.value,
    neverExpire: !row.expiresAt,
    expiresAt: row.expiresAt ? new Date(row.expiresAt) : null,
    notes: row.notes || ''
  }
  editDialogVisible.value = true
}

const handleUpdate = async () => {
  if (editForm.value.maxUses < 1 || editForm.value.defaultDays < 1) {
    ElMessage.warning('请输入有效的数值')
    return
  }
  if (editForm.value.maxUses < editForm.value.usedCount) {
    ElMessage.warning('总量不能小于已使用次数')
    return
  }
  if (!editForm.value.neverExpire && !editForm.value.expiresAt) {
    ElMessage.warning('请设置过期时间或选择永久有效')
    return
  }
  if (!editForm.value.registrationPlanGroup) {
    ElMessage.warning('请选择注册套餐分组')
    return
  }
  const trimmedNotes = editForm.value.notes?.trim() ?? ''
  if (trimmedNotes.length > 500) {
    ElMessage.warning('备注最多 500 字')
    return
  }

  const payload: UpdateRedemptionCodeRequest = {
    maxUses: editForm.value.maxUses,
    defaultDays: editForm.value.defaultDays,
    registrationPlanGroup: editForm.value.registrationPlanGroup,
    expiresAt: editForm.value.neverExpire ? null : editForm.value.expiresAt?.toISOString() || null,
    notes: trimmedNotes || undefined
  }

  editing.value = true
  try {
    await updateRedemptionCode(editForm.value.id, payload)
    ElMessage.success('兑换码更新成功')
    editDialogVisible.value = false
    await fetchData()
  } catch {
    // handled
  } finally {
    editing.value = false
  }
}

const batchCreatedCodesText = computed(() => batchCreatedCodes.value.map((item) => item.code).join('\n'))

const copyBatchCodes = async () => {
  if (!batchCreatedCodesText.value) return
  try {
    await navigator.clipboard.writeText(batchCreatedCodesText.value)
    ElMessage.success('复制成功')
  } catch {
    ElMessage.error('复制失败')
  }
}

const activeFilterCount = computed(() => {
  let count = 0
  if (queryParams.value.code?.trim()) count += 1
  if (queryParams.value.status) count += 1
  if (queryParams.value.registrationPlanGroup) count += 1
  if (queryParams.value.showAll) count += 1
  return count
})

const formatDate = (dateStr?: string | null) => {
  if (!dateStr) return '永久有效'
  return formatDateTime(dateStr, 'short')
}

const formatRegistrationPlanGroupName = (row: RedemptionCode) => {
  return row.registrationPlanGroupName || planGroupNameMap.value.get(row.registrationPlanGroup) || row.registrationPlanGroup
}

const formatRegistrationPlanGroupHint = (row: RedemptionCode) => {
  return row.registrationPlanGroup
}

const getUsageStatus = (row: RedemptionCode) => {
  if (row.usedCount >= row.maxUses) return { type: 'danger', text: '已耗尽' }
  if (row.expiresAt && new Date(row.expiresAt) < new Date()) return { type: 'info', text: '已过期' }
  return { type: 'success', text: '有效' }
}

onMounted(async () => {
  await Promise.all([fetchData(), fetchPlanGroups()])
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      :title="props.embedded ? '兑换码池' : '兑换码管理'"
      :description="props.embedded ? '支持按兑换码、状态和注册套餐分组筛选。' : '生成和管理注册/续期兑换码'"
    >
      <template v-if="!props.embedded" #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">{{ total }} 个兑换码</span>
      </template>

      <template #actions>
        <div class="flex flex-col gap-3 xl:items-end">
          <slot name="tabs" />

          <div class="flex flex-wrap items-center gap-3">
            <div class="flex items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-2.5 py-1.5">
              <span class="text-sm text-gray-600">包含失效/耗尽</span>
              <el-switch v-model="queryParams.showAll" size="small" @change="handleSearch" />
            </div>
            <button
              @click="fetchData"
              class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
              aria-label="刷新兑换码列表"
              title="刷新列表"
            >
              <el-icon :size="18"><Refresh /></el-icon>
            </button>
            <button
              @click="openCreateDialog"
              class="btn-ember inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
            >
              <el-icon><Plus /></el-icon>
              <span>生成兑换码</span>
            </button>
          </div>
        </div>
      </template>

      <div class="flex flex-wrap items-center gap-2">
        <span class="text-sm font-semibold text-gray-900">兑换码池</span>
        <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-500">当前结果 {{ total }} 条</span>
        <span v-if="activeFilterCount > 0" class="rounded-full bg-red-50 px-2.5 py-1 text-xs text-red-600">
          已启用 {{ activeFilterCount }} 个筛选条件
        </span>
      </div>
      <p class="text-sm text-gray-500" :class="props.embedded ? 'mt-0.5' : 'mt-2'">
        支持按兑换码、状态和注册套餐分组筛选。
      </p>

      <EmberFilterPanel
        wrapper-class="flex flex-col gap-3 xl:flex-row xl:items-end"
        content-class="grid flex-1 grid-cols-1 gap-3 md:grid-cols-3"
        actions-class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0"
      >
        <EmberSearchInput
          v-model="queryParams.code"
          label="兑换码"
          aria-label="按兑换码筛选"
          placeholder="输入兑换码关键字"
          :icon="Search"
          @enter="handleSearch"
        />

        <EmberSelectField
          v-model="queryParams.status"
          label="状态"
          placeholder="全部状态"
          clearable
        >
          <el-option
            v-for="item in statusOptions"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </EmberSelectField>

        <EmberSelectField
          v-model="queryParams.registrationPlanGroup"
          label="注册套餐分组"
          placeholder="全部分组"
          clearable
          filterable
          :icon="CollectionTag"
        >
          <el-option
            v-for="option in planGroupOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </EmberSelectField>

        <template #actions>
          <button
            @click="handleReset"
            class="rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            重置
          </button>
          <button
            @click="handleSearch"
            class="btn-ember inline-flex items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
          >
            <el-icon><Search /></el-icon>
            <span>查询</span>
          </button>
        </template>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
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

        <el-table-column label="注册套餐分组" min-width="220">
          <template #default="{ row }">
            <div class="space-y-1">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900">{{ formatRegistrationPlanGroupName(row) }}</span>
              </div>
              <div class="text-xs text-gray-500">{{ formatRegistrationPlanGroupHint(row) }}</div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="备注" min-width="200">
          <template #default="{ row }">
            <span class="text-gray-700">{{ row.notes || '无备注' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="过期时间" min-width="180">
          <template #default="{ row }">
            <span :class="{ 'text-gray-400': !row.expiresAt }">{{ formatDate(row.expiresAt) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <button
              @click="openEditDialog(row)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
              aria-label="编辑兑换码"
              title="编辑"
            >
              <el-icon :size="18"><EditPen /></el-icon>
            </button>
            <button
              @click="handleDelete(row.id)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
              aria-label="删除兑换码"
              title="删除"
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
      title="生成兑换码"
      width="680px"
      class="rounded-2xl"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-5">
          <div class="rounded-2xl border border-gray-200 bg-gray-50/70 p-4">
            <div class="mb-3">
              <div class="text-sm font-semibold text-gray-900">基础规则</div>
              <div class="mt-1 text-xs text-gray-500">批量生成时，所有兑换码共用同一组规则。</div>
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
              <el-form-item label="生成数量" class="mb-0">
                <el-input-number v-model="form.count" :min="1" :max="maxBatchCreateCount" class="w-full !w-full form-number" />
              </el-form-item>
              <el-form-item label="最大使用次数" class="mb-0">
                <el-input-number v-model="form.maxUses" :min="1" class="w-full !w-full form-number" />
              </el-form-item>
              <el-form-item label="有效天数（激活后）" class="mb-0 md:col-span-2 xl:col-span-1">
                <el-input-number v-model="form.defaultDays" :min="1" class="w-full !w-full form-number" />
              </el-form-item>
            </div>

            <div class="mt-3 text-xs text-gray-500">
              单次最多生成 {{ maxBatchCreateCount }} 个兑换码。
            </div>
          </div>

          <el-form-item label="兑换码过期时间（可选）">
            <div class="w-full space-y-2">
              <el-date-picker
                v-model="form.expiresAt"
                type="datetime"
                value-format="YYYY-MM-DDTHH:mm:ssZ"
                placeholder="不填则永久有效"
                :prefix-icon="Calendar"
                clearable
                class="w-full !w-full input-ember form-date"
              />
              <div class="relative z-[1] text-xs text-gray-400">设置兑换码本身的有效期，过期后无法兑换。</div>
            </div>
          </el-form-item>

          <el-form-item label="注册套餐分组">
            <div class="w-full space-y-2">
              <el-select
                v-model="form.registrationPlanGroup"
                placeholder="选择注册后绑定的分组"
                filterable
                class="w-full !w-full form-select"
              >
                <el-option
                  v-for="option in planGroupOptions"
                  :key="option.value"
                  :label="option.label"
                  :value="option.value"
                />
              </el-select>
              <div class="text-xs text-gray-400">
                仅在注册场景生效；续期兑换不会修改用户分组。
              </div>
            </div>
          </el-form-item>

          <el-form-item label="备注（可选）">
            <div class="w-full space-y-2">
              <el-input
                v-model="form.notes"
                type="textarea"
                rows="2"
                maxlength="500"
                show-word-limit
                placeholder="描述兑换码用途，最多 500 字"
                class="w-full !w-full input-ember"
              />
              <div class="text-xs text-gray-400">不填则保持原有行为。</div>
            </div>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="dialogVisible = false"
            class="cursor-pointer rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleCreate"
            :disabled="generating"
            class="btn-ember cursor-pointer rounded-xl px-6 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md disabled:opacity-70"
          >
            {{ generating ? '生成中...' : '确认生成' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="batchResultDialogVisible"
      title="批量生成结果"
      width="560px"
      class="rounded-2xl"
    >
      <div class="p-6 pt-2 space-y-4">
        <div class="flex items-center justify-between gap-4 rounded-xl border border-orange-100 bg-orange-50 px-4 py-3">
          <div>
            <div class="text-sm font-semibold text-gray-900">本次共生成 {{ batchCreatedCodes.length }} 个兑换码</div>
            <div class="text-xs text-gray-500 mt-1">可直接复制后分发，列表中的兑换码已全部写入系统。</div>
          </div>
          <button
            @click="copyBatchCodes"
            class="inline-flex items-center gap-2 px-3 py-2 text-sm font-medium text-orange-700 bg-white border border-orange-200 rounded-lg hover:bg-orange-100 transition-colors"
          >
            <el-icon><CopyDocument /></el-icon>
            <span>复制全部</span>
          </button>
        </div>

        <div class="max-h-80 overflow-y-auto rounded-xl border border-gray-100 bg-gray-50 p-3 space-y-2">
          <div
            v-for="item in batchCreatedCodes"
            :key="item.id"
            class="flex items-center justify-between gap-3 rounded-lg bg-white px-4 py-3 border border-gray-100"
          >
            <span class="text-xs text-gray-400">{{ item.id }}</span>
            <code class="font-mono text-sm font-medium text-gray-900 select-all">{{ item.code }}</code>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="px-6 pb-6 pt-0 flex justify-end gap-3">
          <button
            @click="batchResultDialogVisible = false"
            class="px-4 py-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors font-medium"
          >
            关闭
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="editDialogVisible"
      title="编辑兑换码"
      width="560px"
      class="rounded-2xl"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <div class="grid grid-cols-2 gap-6">
            <el-form-item label="最大使用次数">
              <el-input-number v-model="editForm.maxUses" :min="editForm.usedCount" class="w-full !w-full" />
            </el-form-item>
            <el-form-item label="有效天数 (激活后)">
              <el-input-number v-model="editForm.defaultDays" :min="1" class="w-full !w-full" />
            </el-form-item>
          </div>

          <div class="text-xs text-gray-400 -mt-2">
            已使用 {{ editForm.usedCount }} 次，最大使用次数不能小于该值。
          </div>

          <el-form-item label="永久有效">
            <el-switch v-model="editForm.neverExpire" />
          </el-form-item>

          <el-form-item label="兑换码过期时间">
            <div class="w-full space-y-2">
              <el-date-picker
                v-model="editForm.expiresAt"
                type="datetime"
                placeholder="不填则永久有效"
                :prefix-icon="Calendar"
                :disabled="editForm.neverExpire"
                clearable
                class="w-full !w-full input-ember form-date"
              />
              <div class="relative z-[1] text-xs text-gray-400">修改后状态会按新规则实时生效。</div>
            </div>
          </el-form-item>

          <el-form-item label="注册套餐分组">
            <div class="w-full space-y-2">
              <el-select
                v-model="editForm.registrationPlanGroup"
                placeholder="选择注册后绑定的分组"
                filterable
                class="w-full !w-full form-select"
              >
                <el-option
                  v-for="option in planGroupOptions"
                  :key="option.value"
                  :label="option.label"
                  :value="option.value"
                />
              </el-select>
              <div class="text-xs text-gray-400">
                仅注册时生效；续期兑换不会修改用户分组。
              </div>
            </div>
          </el-form-item>

          <el-form-item label="备注">
            <div class="w-full space-y-2">
              <el-input
                v-model="editForm.notes"
                type="textarea"
                rows="2"
                maxlength="500"
                show-word-limit
                placeholder="更新备注（可选）"
                class="w-full !w-full input-ember"
              />
              <div class="text-xs text-gray-400">清空后保存会移除原备注。</div>
            </div>
          </el-form-item>
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
            :disabled="editing"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md disabled:opacity-70"
          >
            {{ editing ? '保存中...' : '保存修改' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>

<style scoped>
:deep(.el-table) {
  --el-table-header-bg-color: #f9fafb;
}
</style>
