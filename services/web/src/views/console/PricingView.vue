<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { CreditCard, Timer, Refresh, Money } from '@element-plus/icons-vue'
import { createCheckout, getActivePlans, getMyPayments } from '@/api/console'
import type { Payment, PaymentStatus, Plan } from '@/types/api'

const route = useRoute()
const router = useRouter()

const plans = ref<Plan[]>([])
const plansLoading = ref(false)
const buyingPlanID = ref('')

const payments = ref<Payment[]>([])
const paymentTotal = ref(0)
const paymentLoading = ref(false)
const paymentQuery = ref({
  page: 1,
  pageSize: 10
})

const emptyPlans = computed(() => !plansLoading.value && plans.value.length === 0)
const pendingPlanIDs = computed(() => new Set(
  payments.value
    .filter((item) => item.status === 'pending')
    .map((item) => item.planId)
))

const formatPrice = (price: number, currency: string = 'usd') => {
  if (currency.toLowerCase() === 'usd') {
    return `$${(price / 100).toFixed(2)}`
  }
  return `${(price / 100).toFixed(2)} ${currency.toUpperCase()}`
}

const statusMeta = (status: PaymentStatus) => {
  switch (status) {
    case 'completed':
      return { text: '支付成功', type: 'success' as const }
    case 'failed':
      return { text: '支付失败', type: 'danger' as const }
    default:
      return { text: '待支付', type: 'warning' as const }
  }
}

const fetchPlans = async () => {
  plansLoading.value = true
  try {
    const res = await getActivePlans()
    plans.value = res.data || []
  } finally {
    plansLoading.value = false
  }
}

const fetchPayments = async () => {
  paymentLoading.value = true
  try {
    const res = await getMyPayments(paymentQuery.value)
    payments.value = res.data || []
    paymentTotal.value = res.total || 0
  } finally {
    paymentLoading.value = false
  }
}

const refreshAll = async () => {
  await Promise.all([fetchPlans(), fetchPayments()])
}

const redirectToCheckout = async (planID: string) => {
  buyingPlanID.value = planID
  try {
    const res = await createCheckout(planID)
    if (!res.url) {
      ElMessage.error('支付链接为空，请稍后重试')
      return
    }
    window.location.href = res.url
  } finally {
    buyingPlanID.value = ''
  }
}

const handleCheckout = async (plan: Plan) => {
  await redirectToCheckout(plan.id)
}

const handleContinuePayment = async (payment: Payment) => {
  await redirectToCheckout(payment.planId)
}

const consumeQueryState = async () => {
  const success = route.query.success === 'true'
  const canceled = route.query.canceled === 'true'

  if (success) {
    ElMessage.success('支付成功，正在同步你的订阅时长')
  }
  if (canceled) {
    ElMessage.warning('支付已取消，可重新选择方案购买')
  }

  if (success || canceled) {
    const nextQuery = { ...route.query }
    delete nextQuery.success
    delete nextQuery.canceled
    await router.replace({ query: nextQuery })
  }
}

onMounted(async () => {
  await consumeQueryState()
  await refreshAll()
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          订阅购买
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Stripe</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">选择方案后跳转到 Stripe 安全支付页面完成付款</p>
      </div>

      <button
        @click="refreshAll"
        class="p-2 text-gray-500 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
        title="刷新"
      >
        <el-icon :size="20"><Refresh /></el-icon>
      </button>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6" v-loading="plansLoading">
      <div class="mb-5">
        <h2 class="text-lg font-bold text-gray-900">可购买方案</h2>
        <p class="text-sm text-gray-500 mt-1">付款成功后自动延长你的账户有效期</p>
      </div>

      <div v-if="emptyPlans" class="py-14 text-center text-gray-400 border border-dashed border-gray-200 rounded-xl">
        当前暂无可购买方案
      </div>

      <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-5">
        <div
          v-for="plan in plans"
          :key="plan.id"
          class="rounded-2xl border border-gray-100 p-5 bg-gradient-to-b from-white to-gray-50 hover:border-ember/40 hover:shadow-md transition-all"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <h3 class="font-bold text-lg text-gray-900">{{ plan.name }}</h3>
              <p class="text-sm text-gray-500 mt-1 min-h-[36px]">{{ plan.description || '购买后自动延期，无需兑换码' }}</p>
            </div>
            <div class="w-9 h-9 rounded-lg bg-ember/10 text-ember flex items-center justify-center">
              <el-icon><Money /></el-icon>
            </div>
          </div>

          <div class="mt-5 flex items-end gap-2">
            <span class="text-3xl font-extrabold text-gray-900">{{ formatPrice(plan.price, plan.currency) }}</span>
            <span class="text-sm text-gray-400 mb-1">一次性</span>
          </div>

          <div class="mt-3 text-sm text-gray-600 flex items-center gap-1.5">
            <el-icon><Timer /></el-icon>
            <span>增加 {{ plan.days }} 天订阅</span>
          </div>

          <button
            @click="handleCheckout(plan)"
            :disabled="buyingPlanID === plan.id"
            class="mt-5 w-full py-2.5 rounded-lg bg-ember text-white font-bold hover:bg-red-700 transition-colors disabled:opacity-70 disabled:cursor-not-allowed flex items-center justify-center gap-2"
          >
            <el-icon><CreditCard /></el-icon>
            <span>
              {{ buyingPlanID === plan.id ? '跳转中...' : (pendingPlanIDs.has(plan.id) ? '继续支付' : '立即购买') }}
            </span>
          </button>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <div class="px-6 py-4 border-b border-gray-100 flex items-center justify-between">
        <div>
          <h2 class="font-bold text-gray-900">支付记录</h2>
          <p class="text-xs text-gray-500 mt-1">仅展示当前账户的付款历史</p>
        </div>
      </div>

      <el-table
        :data="payments"
        v-loading="paymentLoading"
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
        <el-table-column label="方案" min-width="180">
          <template #default="{ row }">
            <span class="font-medium text-gray-900">{{ row.planName || '未知方案' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="金额" width="120">
          <template #default="{ row }">
            <span class="font-semibold text-gray-900">{{ formatPrice(row.amount, row.currency) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="天数" width="110">
          <template #default="{ row }">
            <span class="text-gray-700">+{{ row.days }} 天</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" effect="light" round size="small">
              {{ statusMeta(row.status).text }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="140">
          <template #default="{ row }">
            <button
              v-if="row.status === 'pending'"
              @click="handleContinuePayment(row)"
              :disabled="buyingPlanID === row.planId"
              class="inline-flex items-center justify-center rounded-lg border border-ember/20 bg-ember/5 px-3 py-1.5 text-xs font-semibold text-ember transition-colors hover:bg-ember/10 disabled:opacity-60"
            >
              {{ buyingPlanID === row.planId ? '跳转中...' : '继续支付' }}
            </button>
            <span v-else class="text-xs text-gray-400">-</span>
          </template>
        </el-table-column>

        <el-table-column label="支付时间" min-width="180">
          <template #default="{ row }">
            <span class="text-gray-600">{{ new Date(row.createdAt).toLocaleString() }}</span>
          </template>
        </el-table-column>
      </el-table>

      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
        <el-pagination
          v-model:current-page="paymentQuery.page"
          v-model:page-size="paymentQuery.pageSize"
          :total="paymentTotal"
          layout="total, prev, pager, next"
          @current-change="fetchPayments"
          background
        />
      </div>
    </div>
  </div>
</template>
