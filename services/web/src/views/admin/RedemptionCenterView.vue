<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Document, Ticket } from '@element-plus/icons-vue'
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
      </template>
    </component>
  </div>
</template>
