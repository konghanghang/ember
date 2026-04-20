<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Document, Ticket } from '@element-plus/icons-vue'
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

const setTab = async (tab: RedemptionTab) => {
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
    <component :is="activeComponent" embedded>
      <template #tabs>
        <EmberSegmentTabs
          :model-value="activeTab"
          :tabs="tabs"
          aria-label="兑换中心分段切换"
          @change="setTab"
        />
      </template>
    </component>
  </div>
</template>
