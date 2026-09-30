<script setup lang="ts">
import { computed, watch } from 'vue'
import type { Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CreditCard, Goods, CollectionTag, Ticket, Document } from '@element-plus/icons-vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import PaymentsView from './PaymentsView.vue'
import PlanGroupsView from './PlanGroupsView.vue'
import PlansView from './PlansView.vue'
import RedemptionCodesView from './RedemptionCodesView.vue'
import RedemptionHistoryView from './RedemptionHistoryView.vue'

type PaymentTab = 'plans' | 'payments' | 'codes' | 'history' | 'groups'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: PaymentTab; label: string; icon: typeof Goods }> = [
  { key: 'plans', label: '付费方案', icon: Goods },
  { key: 'payments', label: '支付记录', icon: CreditCard },
  { key: 'codes', label: '兑换码', icon: Ticket },
  { key: 'history', label: '兑换记录', icon: Document },
  { key: 'groups', label: '套餐分组', icon: CollectionTag }
]

const components: Record<PaymentTab, Component> = {
  plans: PlansView,
  payments: PaymentsView,
  codes: RedemptionCodesView,
  history: RedemptionHistoryView,
  groups: PlanGroupsView,
}

/** 校验 URL 与分段事件的 tab，避免非法值选中不存在的页面。 */
const isPaymentTab = (value: unknown): value is PaymentTab => {
  return typeof value === 'string' && tabs.some(tab => tab.key === value)
}

const activeTab = computed<PaymentTab>(() => (
  isPaymentTab(route.query.tab) ? route.query.tab : 'plans'
))
const activeComponent = computed(() => components[activeTab.value])

/** 切换同级分段，保留现有查询上下文且不重复导航。 */
const setTab = async (tab: string) => {
  if (!isPaymentTab(tab)) return
  if (tab === activeTab.value) return
  await router.replace({
    query: {
      ...route.query,
      tab
    }
  })
}

watch(
  () => route.query.tab,
  async (tab) => {
    if (tab === undefined || isPaymentTab(tab)) return
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
    <EmberPageHeaderCard title="计费中心">
      <template #actions>
        <div class="max-w-full overflow-x-auto">
          <EmberSegmentTabs
            :model-value="activeTab"
            :tabs="tabs"
            ariaLabel="计费中心分段切换"
            @change="setTab"
          />
        </div>
      </template>
    </EmberPageHeaderCard>

    <component :is="activeComponent" embedded />
  </div>
</template>
