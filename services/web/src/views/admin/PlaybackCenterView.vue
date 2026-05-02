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

const setTab = async (tab: PlaybackTab) => {
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
          aria-label="播放分析分段切换"
          @change="setTab"
        />
      </template>
    </component>
  </div>
</template>
