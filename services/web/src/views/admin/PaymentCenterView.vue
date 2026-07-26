<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CreditCard, Goods, CollectionTag } from '@element-plus/icons-vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import PaymentsView from './PaymentsView.vue'
import PlansView from './PlansView.vue'

type PaymentTab = 'plans' | 'payments'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: PaymentTab; label: string; icon: typeof Goods }> = [
  { key: 'plans', label: '付费方案', icon: Goods },
  { key: 'payments', label: '支付记录', icon: CreditCard }
]

const activeTab = computed<PaymentTab>(() => {
  const tab = route.query.tab
  return tab === 'payments' ? 'payments' : 'plans'
})

const activeComponent = computed(() => (
  activeTab.value === 'payments'
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

// 套餐分组是独立路由页（侧边栏另有导航项），不是支付中心的分段；单选分段不做导航，这里只提供入口按钮。
const goPlanGroups = async () => {
  await router.push({ name: 'console-plan-groups' })
}

watch(
  () => route.query.tab,
  async (tab) => {
    if (tab === undefined || tab === 'plans' || tab === 'payments') return
    if (tab === 'groups') {
      // 兼容旧地址 /console/billing?tab=groups → 套餐分组独立页。
      // 该映射是 web-information-architecture.md 明确记录的兼容合同，不可当非法值吞掉。
      await router.replace({ name: 'console-plan-groups' })
      return
    }
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
    <component :is="activeComponent" embedded>
      <template #tabs>
        <div class="flex flex-wrap items-center gap-3">
          <EmberSegmentTabs
            :model-value="activeTab"
            :tabs="tabs"
            ariaLabel="支付中心分段切换"
            @change="setTab"
          />
          <button
            @click="goPlanGroups"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            <el-icon><CollectionTag /></el-icon>
            <span>套餐分组</span>
          </button>
        </div>
      </template>
    </component>
  </div>
</template>
