<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { resolvePayment } from '@/api/entitlements'
import { getPlanGroups } from '@/api/admin'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import PlanBenefitsEditor from '@/components/billing/PlanBenefitsEditor.vue'
import { benefitText, validateBenefits } from '@/utils/entitlements'
import type { PlanBenefit, ManagedPlanGroup } from '@/types/api'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, RefreshRight, UserFilled, CreditCard, Goods, CollectionTag } from '@element-plus/icons-vue'
import { getAllPayments, getPlans } from '@/api/admin'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import { formatDate } from '@/utils/date'
import type { Payment, PaymentStatus, Plan } from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const tableData = ref<Payment[]>([])
const total = ref(0)

const queryParams = ref({
  page: 1,
  pageSize: 20,
  userId: '',
  planId: '',
  status: '' as PaymentStatus | ''
})

const activeUserId = computed(() => queryParams.value.userId.trim())
const activePlanId = computed(() => queryParams.value.planId.trim())
const activeStatus = computed(() => queryParams.value.status)
const planOptions = ref<Plan[]>([])
const statusOptions: Array<{ label: string; value: PaymentStatus }> = [
  { label: '待人工处理', value: 'paid_review' },
  { label: '已人工处理', value: 'resolved' },
  { label: '待支付', value: 'pending' },
  { label: '支付成功', value: 'completed' },
  { label: '已过期', value: 'expired' },
  { label: '支付失败', value: 'failed' }
]

const formatPrice = (price: number, currency: string = 'usd') => {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: currency.toUpperCase(),
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(price / 100)
}

const statusMeta = (status: PaymentStatus) => {
  switch (status) {
    case 'paid_review':
      return { text: '待人工处理', type: 'warning' as const }
    case 'resolved':
      return { text: '已人工处理', type: 'info' as const }
    case 'completed':
      return { text: '支付成功', type: 'success' as const }
    case 'expired':
      return { text: '已过期', type: 'danger' as const }
    case 'failed':
      return { text: '支付失败', type: 'danger' as const }
    default:
      return { text: '待支付', type: 'warning' as const }
  }
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getAllPayments({
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize,
      userId: activeUserId.value || undefined,
      planId: activePlanId.value || undefined,
      status: activeStatus.value || undefined
    })
    tableData.value = res.data || []
    total.value = res.total || 0
  } catch {
    // 全局错误拦截已处理
  } finally {
    loading.value = false
  }
}

const fetchPlans = async () => {
  try {
    const res = await getPlans({ page: 1, pageSize: 100, showAll: true })
    planOptions.value = res.data || []
  } catch {
    planOptions.value = []
  }
}

const syncRouteUserID = async () => {
  const nextUserID = activeUserId.value
  const currentUserID = typeof route.query.userId === 'string' ? route.query.userId.trim() : ''
  const currentPlanID = typeof route.query.planId === 'string' ? route.query.planId.trim() : ''
  const currentStatus = typeof route.query.status === 'string' ? route.query.status.trim() : ''
  const nextQuery = { ...route.query }

  if (nextUserID) {
    nextQuery.userId = nextUserID
  } else {
    delete nextQuery.userId
  }
  if (activePlanId.value) {
    nextQuery.planId = activePlanId.value
  } else {
    delete nextQuery.planId
  }
  if (activeStatus.value) {
    nextQuery.status = activeStatus.value
  } else {
    delete nextQuery.status
  }

  if (
    nextUserID === currentUserID &&
    activePlanId.value === currentPlanID &&
    activeStatus.value === currentStatus
  ) {
    await fetchData()
    return
  }

  await router.replace({ query: nextQuery })
}

const handleSearch = async () => {
  queryParams.value.page = 1
  await syncRouteUserID()
}

const handleReset = async () => {
  queryParams.value.page = 1
  queryParams.value.pageSize = 20
  queryParams.value.userId = ''
  queryParams.value.planId = ''
  queryParams.value.status = ''
  await syncRouteUserID()
}

const handlePageSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

watch(
  () => [route.query.userId, route.query.planId, route.query.status],
  ([userId, planId, status]) => {
    const nextUserID = typeof userId === 'string' ? userId.trim() : ''
    const nextPlanID = typeof planId === 'string' ? planId.trim() : ''
    const nextStatus = typeof status === 'string' ? status.trim() as PaymentStatus | '' : ''

    if (queryParams.value.userId !== nextUserID) {
      queryParams.value.userId = nextUserID
      queryParams.value.page = 1
    }
    if (queryParams.value.planId !== nextPlanID) {
      queryParams.value.planId = nextPlanID
      queryParams.value.page = 1
    }
    if (queryParams.value.status !== nextStatus) {
      queryParams.value.status = nextStatus
      queryParams.value.page = 1
    }
    fetchData()
  },
  { immediate: true }
)

onMounted(fetchPlans)
const reviewPayment = ref<Payment | null>(null)
const resolution = ref<'external_refund' | 'compensation'>('external_refund')
const resolutionNote = ref('')
const compensationBenefits = ref<PlanBenefit[]>([])
const reviewGroups = ref<ManagedPlanGroup[]>([])
const resolving = ref(false)
/** 展示已付款异常原因，补偿配置复用套餐权益编辑器。 */
async function openReview(payment: Payment) {
  reviewGroups.value = (await getPlanGroups()).data
  resolution.value = 'external_refund'
  resolutionNote.value = ''
  compensationBenefits.value = []
  reviewPayment.value = payment
}
/** 退款仅记录管理员已在线下完成的结果，不声称此按钮执行退款。 */
async function submitReview() {
  if (!reviewPayment.value || resolving.value) return
  if (!resolutionNote.value.trim()) { ElMessage.warning('请填写处理记录'); return }
  if (resolution.value === 'compensation') { const error = validateBenefits(compensationBenefits.value); if (error) { ElMessage.warning(error); return } }
  resolving.value = true
  try {
    await resolvePayment(reviewPayment.value.id, resolution.value, resolutionNote.value.trim(), compensationBenefits.value)
    reviewPayment.value = null
    ElMessage.success('人工处理已记录')
    await fetchData()
  } finally { resolving.value = false }
}
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard :hide-title="props.embedded" title="支付记录">
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">当前结果 {{ total }} 条</span>
      </template>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_auto]"
        content-class="grid grid-cols-1 gap-3 lg:grid-cols-3"
      >
        <EmberSearchInput
          v-model="queryParams.userId"
          label="用户 ID"
          type="text"
          inputmode="text"
          aria-label="按用户 ID 筛选"
          placeholder="输入完整用户 ID"
          :icon="UserFilled"
          @enter="handleSearch"
        />

        <EmberSelectField
          v-model="queryParams.planId"
          label="付费方案"
          placeholder="全部方案"
          clearable
          filterable
          :icon="Goods"
        >
          <el-option
            v-for="plan in planOptions"
            :key="plan.id"
            :label="plan.name"
            :value="plan.id"
          />
        </EmberSelectField>

        <EmberSelectField
          v-model="queryParams.status"
          label="支付状态"
          placeholder="全部状态"
          clearable
          :icon="CollectionTag"
        >
          <el-option
            v-for="option in statusOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </EmberSelectField>

        <template #actions>
          <button
            @click="handleReset"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            <el-icon><RefreshRight /></el-icon>
            重置
          </button>
          <button
            @click="handleSearch"
            class="btn-ember inline-flex cursor-pointer items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
          >
            <el-icon><Search /></el-icon>
            查询
          </button>
        </template>
      </EmberFilterPanel>

      <div
        v-if="activeUserId"
        class="mt-4 inline-flex items-center gap-2 rounded-full border border-ember/20 bg-ember/5 px-3 py-1.5 text-sm text-ember"
      >
        <el-icon><CreditCard /></el-icon>
        <span>当前仅查看用户 {{ activeUserId }} 的支付记录</span>
      </div>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
      <el-table-column label="用户" min-width="220">
        <template #default="{ row }">
          <div class="space-y-1">
            <div class="font-semibold text-gray-900">{{ row.username || '-' }}</div>
            <code class="text-xs text-gray-500">{{ row.userId }}</code>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="方案" min-width="180">
        <template #default="{ row }">
          <span class="font-medium text-gray-900">{{ row.planName || '未知方案' }}</span>
        </template>
      </el-table-column>

      <el-table-column label="金额" width="140">
        <template #default="{ row }">
          <span class="font-semibold text-gray-900">{{ formatPrice(row.amount, row.currency) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="购买权益" min-width="180"><template #default="{ row }"><div v-for="benefit in row.benefits" :key="benefit.planGroup">{{ benefitText(benefit) }}</div><span v-if="!row.benefits?.length">历史订单 · {{ row.days }} 天</span></template></el-table-column>

      <el-table-column label="状态" width="120" align="center">
        <template #default="{ row }">
          <el-tag :type="statusMeta(row.status).type" effect="light" round size="small">
            {{ statusMeta(row.status).text }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column label="人工处理" min-width="200"><template #default="{ row }">
        <button v-if="row.status === 'paid_review'" class="btn-ember px-3 py-2" @click="openReview(row)">处理订单</button>
        <span v-else>{{ row.resolutionNote || '—' }}</span>
      </template></el-table-column>
      <el-table-column prop="stripeSessionId" label="Session ID" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          <code class="text-xs text-gray-600">{{ row.stripeSessionId || '-' }}</code>
        </template>
      </el-table-column>

      <el-table-column prop="stripePaymentIntentId" label="Payment Intent" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          <code class="text-xs text-gray-600">{{ row.stripePaymentIntentId || '-' }}</code>
        </template>
      </el-table-column>

      <el-table-column label="支付时间" min-width="170">
        <template #default="{ row }">
          <span class="text-gray-600">{{ formatDate(row.createdAt) }}</span>
        </template>
      </el-table-column>

      <template #pagination>
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="fetchData"
          @size-change="handlePageSizeChange"
          background
        />
      </template>
    </EmberTableCard>
  </div>
  <EmberFormDialog :model-value="!!reviewPayment" title="处理已付款订单" width="680px" @update:model-value="value => { if (!value) reviewPayment = null }">
    <el-form label-position="top" class="space-y-4 p-6">
      <p class="text-sm text-gray-600">{{ reviewPayment?.manualReviewReason === 'already_owned' ? '付款时已拥有对应永久权益' : '订单权益信息无法确定，需要人工核对' }}</p>
      <el-form-item label="处理方式"><el-select v-model="resolution" class="form-select w-full"><el-option label="记录线下已退款" value="external_refund" /><el-option label="发放补偿权益" value="compensation" /></el-select></el-form-item>
      <PlanBenefitsEditor v-if="resolution === 'compensation'" v-model="compensationBenefits" :groups="reviewGroups" />
      <el-form-item label="处理记录"><el-input v-model="resolutionNote" type="textarea" :maxlength="500" class="input-ember" /></el-form-item>
    </el-form>
    <template #footer><button class="btn-ember px-4 py-2.5" :disabled="resolving" @click="submitReview">{{ resolving ? '处理中…' : '确认处理' }}</button></template>
  </EmberFormDialog>
</template>
