<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, RefreshRight, UserFilled, CreditCard } from '@element-plus/icons-vue'
import { getAllPayments, getPlans } from '@/api/admin'
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
    case 'completed':
      return { text: '支付成功', type: 'success' as const }
    case 'expired':
      return { text: '已过期', type: 'info' as const }
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
</script>

<template>
  <div class="space-y-6">
    <div v-if="!props.embedded" class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          支付记录
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Total: {{ total }}</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">管理员审计全部支付记录，当前支持按用户 ID 精确筛选</p>
      </div>

      <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
        <div class="grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_auto] gap-3">
          <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">
            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">用户 ID</label>
              <div class="relative group">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><UserFilled /></el-icon>
                </div>
                <input
                  v-model="queryParams.userId"
                  type="text"
                  autocomplete="off"
                  placeholder="输入完整用户 ID"
                  class="filter-input w-full pl-10 pr-4"
                  @keyup.enter="handleSearch"
                />
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">付费方案</label>
              <el-select
                v-model="queryParams.planId"
                placeholder="全部方案"
                clearable
                filterable
                class="w-full"
              >
                <el-option
                  v-for="plan in planOptions"
                  :key="plan.id"
                  :label="plan.name"
                  :value="plan.id"
                />
              </el-select>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">支付状态</label>
              <el-select
                v-model="queryParams.status"
                placeholder="全部状态"
                clearable
                class="w-full"
              >
                <el-option
                  v-for="option in statusOptions"
                  :key="option.value"
                  :label="option.label"
                  :value="option.value"
                />
              </el-select>
            </div>
          </div>

          <div class="flex items-end justify-end gap-2">
            <button
              @click="handleReset"
              class="px-4 py-2.5 text-sm text-gray-700 bg-white border border-gray-200 hover:bg-gray-100 rounded-xl transition-colors cursor-pointer inline-flex items-center gap-1.5"
            >
              <el-icon><RefreshRight /></el-icon>
              重置
            </button>
            <button
              @click="handleSearch"
              class="btn-ember px-4 py-2.5 text-sm rounded-xl font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer inline-flex items-center gap-1.5"
            >
              <el-icon><Search /></el-icon>
              查询
            </button>
          </div>
        </div>
      </div>

      <div v-if="activeUserId" class="mt-4 inline-flex items-center gap-2 rounded-full border border-ember/20 bg-ember/5 px-3 py-1.5 text-sm text-ember">
        <el-icon><CreditCard /></el-icon>
        <span>当前仅查看用户 {{ activeUserId }} 的支付记录</span>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
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

        <el-table-column label="天数" width="100" align="center">
          <template #default="{ row }">
            <span class="text-gray-700">+{{ row.days }} 天</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" effect="light" round size="small">
              {{ statusMeta(row.status).text }}
            </el-tag>
          </template>
        </el-table-column>

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
      </el-table>

      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
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
      </div>
    </div>
  </div>
</template>

<style scoped>
.filter-input {
  background-color: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 0.75rem;
  height: 42px;
  line-height: 1.2;
  font-size: 0.875rem;
  color: #111827;
  outline: none;
  transition: all 0.2s ease;
}

.filter-input::placeholder {
  color: #9ca3af;
}

.filter-input:hover {
  background-color: #ffffff;
}

.filter-input:focus {
  background-color: #ffffff;
  border-color: var(--ember-red);
  box-shadow: 0 0 0 4px rgba(229, 9, 20, 0.1);
}
</style>
