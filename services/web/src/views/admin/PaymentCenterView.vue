<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CreditCard, Goods, CollectionTag } from '@element-plus/icons-vue'
import PaymentsView from './PaymentsView.vue'
import PlansView from './PlansView.vue'
import PlanGroupsView from './PlanGroupsView.vue'

type PaymentTab = 'groups' | 'plans' | 'payments'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: PaymentTab; label: string; icon: typeof Goods }> = [
  { key: 'groups', label: '套餐分组', icon: CollectionTag },
  { key: 'plans', label: '付费方案', icon: Goods },
  { key: 'payments', label: '支付记录', icon: CreditCard }
]

const activeTab = computed<PaymentTab>(() => {
  const tab = route.query.tab
  if (tab === 'payments' || tab === 'plans' || tab === 'groups') return tab
  return 'plans'
})

const activeComponent = computed(() => (
  activeTab.value === 'payments'
    ? PaymentsView
    : activeTab.value === 'plans'
      ? PlansView
      : PlanGroupsView
))

const setTab = async (tab: PaymentTab) => {
  if (tab === activeTab.value) return
  const nextQuery = { ...route.query, tab }
  await router.replace({ query: nextQuery })
}

watch(
  () => route.query.tab,
  async (tab) => {
    if (tab === undefined || tab === 'groups' || tab === 'plans' || tab === 'payments') return
    await router.replace({
      query: {
        ...route.query,
        tab: 'plans'
      }
    })
  },
  { immediate: true }
)
</script>

<template>
  <div class="space-y-6">
    <section class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-slate-900">支付中心</h1>
          <p class="mt-1 text-sm text-slate-500">把方案配置和订单审计放在一起，支付工作流更完整。</p>
        </div>

        <div class="inline-flex w-full rounded-2xl bg-slate-100 p-1 lg:w-auto">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            class="flex flex-1 items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors lg:flex-none"
            :class="activeTab === tab.key ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500 hover:text-slate-700'"
            @click="setTab(tab.key)"
          >
            <el-icon><component :is="tab.icon" /></el-icon>
            <span>{{ tab.label }}</span>
          </button>
        </div>
      </div>
    </section>

    <component :is="activeComponent" embedded />
  </div>
</template>
