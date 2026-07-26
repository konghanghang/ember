<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { DataLine, Document } from '@element-plus/icons-vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import UserPlaybackProfilesView from './UserPlaybackProfilesView.vue'
import PlaybackHistoryView from './PlaybackHistoryView.vue'

type PlaybackTab = 'profiles' | 'history'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: PlaybackTab; label: string; icon: typeof DataLine }> = [
  { key: 'profiles', label: '用户画像', icon: DataLine },
  { key: 'history', label: '播放历史', icon: Document }
]

const activeTab = computed<PlaybackTab>(() => {
  const tab = route.query.tab
  return tab === 'history' ? 'history' : 'profiles'
})

const activeComponent = computed(() => (
  activeTab.value === 'history' ? PlaybackHistoryView : UserPlaybackProfilesView
))

// EmberSegmentTabs 的 change 事件按 string 发出；这里收窄回已知 tab 集合，非法值直接忽略。
const isPlaybackTab = (value: string): value is PlaybackTab => {
  return tabs.some((tab) => tab.key === value)
}

const setTab = async (tab: string) => {
  if (!isPlaybackTab(tab)) return
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
    if (tab === undefined || tab === 'profiles' || tab === 'history') return
    await router.replace({
      query: {
        ...route.query,
        tab: 'profiles'
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
          ariaLabel="播放分析分段切换"
          @change="setTab"
        />
      </template>
    </component>
  </div>
</template>
