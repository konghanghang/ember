<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CreditCard, Goods, CollectionTag } from '@element-plus/icons-vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import PaymentsView from './PaymentsView.vue'
import PlanGroupsView from './PlanGroupsView.vue'
import PlansView from './PlansView.vue'

type PaymentTab = 'plans' | 'payments' | 'groups'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: PaymentTab; label: string; icon: typeof Goods }> = [
  { key: 'plans', label: '付费方案', icon: Goods },
  { key: 'payments', label: '支付记录', icon: CreditCard },
  { key: 'groups', label: '套餐分组', icon: CollectionTag }
]

const activeTab = computed<PaymentTab>(() => {
  const tab = route.query.tab
  if (tab === 'groups') return 'groups'
  return tab === 'payments' ? 'payments' : 'plans'
})

const activeComponent = computed(() => (
  activeTab.value === 'groups'
    ? PlanGroupsView
    : activeTab.value === 'payments'
      ? PaymentsView
      : PlansView
))

// EmberSegmentTabs 的 change 事件按 string 发出；这里收窄回已知 tab 集合，非法值直接忽略。
const isPaymentTab = (value: string): value is PaymentTab => {
  return tabs.some((tab) => tab.key === value)
}

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
    if (tab === undefined || tab === 'plans' || tab === 'payments' || tab === 'groups') return
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
