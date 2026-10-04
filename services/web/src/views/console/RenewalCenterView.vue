<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { CreditCard, Money, Ticket, Clock } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import { createCheckout, getActivePlans, getMyPayments } from '@/api/console'
import { refreshConsoleProfileKey, type RefreshConsoleProfile } from '@/constants/consoleProfile'
import { getRedemptions, redeemCode } from '@/api/user'
import { formatDate } from '@/utils/date'
import type { Payment, PaymentStatus, Plan, Redemption } from '@/types/api'

type RenewalTab = 'online' | 'redeem'

const route = useRoute()
const router = useRouter()
const refreshProfile = inject<RefreshConsoleProfile>(refreshConsoleProfileKey, async () => {})

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

const redemptions = ref<Redemption[]>([])
const redemptionsLoading = ref(false)
const redemptionTotal = ref(0)
const redemptionQuery = ref({
  page: 1,
  pageSize: 10
})
let paymentRequestToken = 0
let redemptionRequestToken = 0

const redeemForm = ref({ code: '' })
const redeeming = ref(false)
const activeRenewalTab = ref<RenewalTab>('online')
const activeHistoryTab = ref<'payments' | 'redemptions'>('payments')
const emptyPlans = computed(() => !plansLoading.value && plans.value.length === 0)
const pendingPlanIDs = computed(() => new Set(
  payments.value
    .filter((item) => item.status === 'pending')
    .map((item) => item.planId)
))
const renewalTabs: Array<{ key: RenewalTab; label: string }> = [
  { key: 'online', label: '在线购买' },
  { key: 'redeem', label: '兑换码续期' }
]
const renewalSegmentTabs = computed(() => renewalTabs.map((tab) => ({ key: tab.key, label: tab.label })))
const historySegmentTabs = computed(() => [
  { key: 'payments', label: `支付记录 ${paymentTotal.value}` },
  { key: 'redemptions', label: `兑换记录 ${redemptionTotal.value}` }
])

const formatPrice = (price: number, currency: string = 'usd') => {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: currency.toUpperCase(),
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(price / 100)
}

const paymentStatusMeta = (status: PaymentStatus) => {
  switch (status) {
    case 'paid_review':
      return { text: '已付款 · 待人工处理', type: 'warning' as const }
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
  const requestToken = ++paymentRequestToken
  paymentLoading.value = true
  try {
    const res = await getMyPayments(paymentQuery.value)
    if (requestToken !== paymentRequestToken) return
    payments.value = res.data || []
    paymentTotal.value = res.total || 0
  } finally {
    if (requestToken === paymentRequestToken) {
      paymentLoading.value = false
    }
  }
}

const fetchRedemptions = async () => {
  const requestToken = ++redemptionRequestToken
  redemptionsLoading.value = true
  try {
    const res = await getRedemptions(redemptionQuery.value)
    if (requestToken !== redemptionRequestToken) return
    redemptions.value = res.data || []
    redemptionTotal.value = res.total || 0
  } finally {
    if (requestToken === redemptionRequestToken) {
      redemptionsLoading.value = false
    }
  }
}

const refreshAll = async () => {
  const tasks: Promise<void>[] = [refreshProfile(), fetchPlans(), fetchPayments(), fetchRedemptions()]
  await Promise.all(tasks)
}

const redirectToCheckout = async (planID: string) => {
  buyingPlanID.value = planID
  let redirectStarted = false
  try {
    const res = await createCheckout(planID)
    if (!res.url) {
      // 后端未返回支付链接属于业务异常；request 拦截器只弹通用 error，这里需要差异化文案。
      ElMessage.error('支付链接为空，请稍后重试')
      return
    }
    redirectStarted = true
    window.location.assign(res.url)
  } catch {
    // 创建支付链接失败由 request 拦截器统一弹后端 error 文案，这里只复位按钮状态。
  } finally {
    if (!redirectStarted) {
      buyingPlanID.value = ''
    }
  }
}

/** 服务端决定购买资格，按钮和处理入口一致禁止无增益购买。 */
const handleCheckout = async (plan: Plan) => {
  if (plan.purchasable === false || buyingPlanID.value) return
  await redirectToCheckout(plan.id)
}

const handleContinuePayment = async (payment: Payment) => {
  await redirectToCheckout(payment.planId)
}

const handleRedeem = async () => {
  if (!redeemForm.value.code.trim()) {
    ElMessage.warning('请输入兑换码')
    return
  }

  redeeming.value = true
  try {
    const result = await redeemCode({ code: redeemForm.value.code.trim() })
    ElMessage.success(result.message)
    redeemForm.value.code = ''
    await Promise.all([refreshProfile(), fetchRedemptions()])
  } catch {
    // handled by interceptor
  } finally {
    redeeming.value = false
  }
}

const handleRedemptionPageSizeChange = (size: number) => {
  redemptionQuery.value.pageSize = size
  redemptionQuery.value.page = 1
  fetchRedemptions()
}

const handlePaymentPageSizeChange = (size: number) => {
  paymentQuery.value.pageSize = size
  paymentQuery.value.page = 1
  fetchPayments()
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
    <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
      <div class="border-b border-gray-100 bg-gray-50/50 px-6 py-5 md:px-7">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h2 class="text-2xl font-bold text-gray-900">续费方式</h2>
            <p class="mt-1 text-sm text-gray-500">选择在线购买或使用兑换码完成续费</p>
          </div>

          <EmberSegmentTabs
            v-model="activeRenewalTab"
            :tabs="renewalSegmentTabs"
            :full-width="false"
            ariaLabel="续费方式切换"
          />
        </div>
      </div>

      <div class="p-4 md:p-7">
        <template v-if="activeRenewalTab === 'online'">
          <div class="flex flex-col gap-6" v-loading="plansLoading">
            <div>
              <div class="mb-4 flex items-center justify-between gap-3">
                <h4 class="text-base font-bold text-gray-900">可购买方案</h4>
                <div class="hidden rounded-full bg-gray-100 px-3 py-1 text-xs font-semibold text-gray-500 md:inline-flex">
                  {{ plans.length }} 个方案
                </div>
              </div>

              <EmberEmptyStateCard
                v-if="emptyPlans"
                title="当前暂无可购买方案"
                description="请稍后刷新，或联系管理员检查付费方案配置。"
              />

              <div v-else class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                <div
                  v-for="plan in plans"
                  :key="plan.id"
                  class="flex h-full min-w-0 flex-col rounded-2xl border border-gray-100 bg-white p-5 transition-colors hover:border-ember/40 md:min-h-[22rem] md:p-6"
                >
                  <div class="min-w-0">
                    <div class="flex items-start justify-between gap-3">
                      <h3 class="min-w-0 break-words text-xl font-bold leading-7 text-gray-900">{{ plan.name }}</h3>
                      <div aria-hidden="true" class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-ember/10 text-ember">
                        <el-icon><Money /></el-icon>
                      </div>
                    </div>
                    <p v-if="plan.description" class="mt-4 whitespace-pre-line break-words rounded-xl bg-ember/5 px-4 py-3 text-base font-medium leading-relaxed text-gray-700 ring-1 ring-ember/10">{{ plan.description }}</p>
                  </div>

                  <div class="mt-auto pt-6">
                    <div class="mb-5">
                      <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                        <span class="break-all text-3xl font-extrabold text-gray-900">{{ formatPrice(plan.price, plan.currency) }}</span>
                        <span class="text-sm text-gray-600">{{ plan.validityType === 'permanent' ? '· 永久有效' : `/ ${plan.days} 天` }}</span>
                      </div>
                    </div>

                    <dl class="mb-4 flex items-start justify-between gap-4 border-t border-gray-100 pt-4 text-sm">
                      <dt class="shrink-0 text-gray-500">所属分组</dt>
                      <dd class="min-w-0 break-words text-right text-gray-600">{{ plan.planGroupName || plan.planGroup }}</dd>
                    </dl>

                    <button
                      @click="handleCheckout(plan)"
                      :disabled="!!buyingPlanID || plan.purchasable === false"
                      class="btn-ember flex w-full cursor-pointer items-center justify-center gap-2 rounded-xl py-3 disabled:cursor-not-allowed disabled:opacity-70"
                    >
                      <el-icon><CreditCard /></el-icon>
                      <span>{{ buyingPlanID === plan.id ? '跳转中...' : (plan.purchasable === false ? (plan.purchaseReason === '已拥有对应永久权益' ? '已拥有' : '暂不可购买') : (pendingPlanIDs.has(plan.id) ? '继续支付' : '立即购买')) }}</span>
                    </button>
                    <p v-if="plan.purchaseReason" class="mt-2 text-sm text-gray-500">{{ plan.purchaseReason }}</p>
                  </div>
                </div>
              </div>
            </div>

          </div>
        </template>

        <template v-else>
          <div class="flex flex-col gap-6">
            <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
              <div class="mb-4">
                <h4 class="text-base font-bold text-gray-900">输入兑换码</h4>
                <p class="mt-1 text-sm text-gray-500">每人每码仅可成功兑换一次，兑换后增加对应分组的有效期。</p>
              </div>

              <div class="rounded-2xl border border-gray-200 bg-gray-50/60 p-5">
                <div class="flex items-start gap-4">
                  <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-white text-ember shadow-sm ring-1 ring-ember/10">
                    <el-icon :size="22"><Ticket /></el-icon>
                  </div>

                  <el-input
                    v-model="redeemForm.code"
                    placeholder="在此输入兑换码"
                    class="input-ember"
                    size="large"
                  />
                </div>

                <button
                  @click="handleRedeem"
                  :disabled="redeeming"
                  class="btn-ember mt-4 flex w-full cursor-pointer items-center justify-center gap-2 rounded-xl py-3 font-bold disabled:opacity-70"
                >
                  <span v-if="redeeming" class="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
                  <span>{{ redeeming ? '验证中...' : '确认兑换' }}</span>
                </button>
              </div>
            </div>
          </div>
        </template>
      </div>
    </section>

    <section class="space-y-4">
      <div class="rounded-2xl border border-gray-100 bg-white px-6 py-4 shadow-sm">
        <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <h2 class="text-2xl font-bold text-gray-900">历史记录</h2>

          <EmberSegmentTabs
            v-model="activeHistoryTab"
            :tabs="historySegmentTabs"
            :full-width="false"
            ariaLabel="续费历史类型切换"
          />
        </div>
      </div>

      <EmberTableCard
        v-if="activeHistoryTab === 'payments'"
        :data="payments"
        :loading="paymentLoading"
      >
        <el-table-column label="方案" min-width="170">
          <template #default="{ row }">
            <span class="font-medium text-gray-900">{{ row.planName || '未知方案' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="金额" width="120">
          <template #default="{ row }">
            <span class="font-semibold text-gray-900">{{ formatPrice(row.amount, row.currency) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="paymentStatusMeta(row.status).type" effect="light" round size="small">
              {{ paymentStatusMeta(row.status).text }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="140">
          <template #default="{ row }">
            <button
              v-if="row.status === 'pending'"
              @click="handleContinuePayment(row)"
              :disabled="buyingPlanID === row.planId"
              class="inline-flex cursor-pointer items-center justify-center rounded-lg border border-ember/20 bg-ember/5 px-3 py-1.5 text-xs font-semibold text-ember transition-colors hover:bg-ember/10 disabled:opacity-60"
            >
              {{ buyingPlanID === row.planId ? '跳转中...' : '继续支付' }}
            </button>
            <span v-else class="text-xs text-gray-400">-</span>
          </template>
        </el-table-column>

        <el-table-column label="时间" min-width="170">
          <template #default="{ row }">
            <span class="text-gray-600">{{ formatDate(row.createdAt) }}</span>
          </template>
        </el-table-column>

        <template #pagination>
          <el-pagination
            v-model:current-page="paymentQuery.page"
            v-model:page-size="paymentQuery.pageSize"
            :total="paymentTotal"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            @current-change="fetchPayments"
            @size-change="handlePaymentPageSizeChange"
            background
          />
        </template>
      </EmberTableCard>

      <EmberTableCard
        v-else
        :data="redemptions"
        :loading="redemptionsLoading"
      >
        <el-table-column prop="code" label="兑换码" min-width="180" />
        <el-table-column label="延长时长" width="120">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1 text-gray-700">
              <el-icon class="text-emerald-600"><Clock /></el-icon>
              <span>+{{ row.days }} 天</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="时间" min-width="170">
          <template #default="{ row }">
            <span class="text-gray-600">{{ formatDate(row.createdAt) }}</span>
          </template>
        </el-table-column>

        <template #pagination>
          <el-pagination
            v-model:current-page="redemptionQuery.page"
            v-model:page-size="redemptionQuery.pageSize"
            :total="redemptionTotal"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            @current-change="fetchRedemptions"
            @size-change="handleRedemptionPageSizeChange"
            background
          />
        </template>
      </EmberTableCard>
    </section>

  </div>
</template>
