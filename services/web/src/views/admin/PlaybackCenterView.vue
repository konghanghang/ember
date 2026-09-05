<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { DataLine, Document, Monitor } from '@element-plus/icons-vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import UserPlaybackProfilesView from './UserPlaybackProfilesView.vue'
import PlaybackHistoryView from './PlaybackHistoryView.vue'
import SessionsView from './SessionsView.vue'

type PlaybackTab = 'sessions' | 'profiles' | 'history'

const route = useRoute()
const router = useRouter()

const tabs: Array<{ key: PlaybackTab; label: string; icon: typeof DataLine }> = [
  { key: 'sessions', label: '实时会话', icon: Monitor },
  { key: 'profiles', label: '用户画像', icon: DataLine },
  { key: 'history', label: '播放历史', icon: Document }
]

const activeTab = computed<PlaybackTab>(() => {
  const tab = route.query.tab
  if (tab === 'history') return 'history'
  if (tab === 'sessions') return 'sessions'
  return 'profiles'
})

const activeComponent = computed(() => (
  activeTab.value === 'history' ? PlaybackHistoryView : activeTab.value === 'sessions' ? SessionsView : UserPlaybackProfilesView
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
    if (tab === undefined || tab === 'profiles' || tab === 'history' || tab === 'sessions') return
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
    <EmberPageHeaderCard title="播放中心">
      <template #actions>
        <div class="max-w-full overflow-x-auto">
          <EmberSegmentTabs
            :model-value="activeTab"
            :tabs="tabs"
            ariaLabel="播放中心分段切换"
            @change="setTab"
          />
        </div>
      </template>
    </EmberPageHeaderCard>

    <component :is="activeComponent" embedded />
  </div>
</template>
