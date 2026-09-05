<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Document, Ticket } from '@element-plus/icons-vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import RedemptionCodesView from './RedemptionCodesView.vue'
import RedemptionHistoryView from './RedemptionHistoryView.vue'

type RedemptionTab = 'codes' | 'history'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: RedemptionTab; label: string; icon: typeof Ticket }> = [
  { key: 'codes', label: '兑换码池', icon: Ticket },
  { key: 'history', label: '兑换记录', icon: Document }
]

const activeTab = computed<RedemptionTab>(() => {
  const tab = route.query.tab
  return tab === 'history' ? 'history' : 'codes'
})

const activeComponent = computed(() => (
  activeTab.value === 'history' ? RedemptionHistoryView : RedemptionCodesView
))

// EmberSegmentTabs 的 change 事件按 string 发出；这里收窄回已知 tab 集合，非法值直接忽略。
const isRedemptionTab = (value: string): value is RedemptionTab => {
  return tabs.some((tab) => tab.key === value)
}

const setTab = async (tab: string) => {
  if (!isRedemptionTab(tab)) return
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
    if (tab === undefined || tab === 'codes' || tab === 'history') return
    await router.replace({
      query: {
        ...route.query,
        tab: 'codes'
      }
    })
  },
  { immediate: true }
)
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard title="兑换中心">
      <template #actions>
        <div class="max-w-full overflow-x-auto">
          <EmberSegmentTabs
            :model-value="activeTab"
            :tabs="tabs"
            ariaLabel="兑换中心分段切换"
            @change="setTab"
          />
        </div>
      </template>
    </EmberPageHeaderCard>

    <component :is="activeComponent" embedded />
  </div>
</template>
