<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Film, VideoPlay, RefreshRight } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import { useUserStore } from '@/store/user'
import { getLatestMedia } from '@/api/console'
import type { LatestMediaItem } from '@/types/api'

const userStore = useUserStore()

const activeType = ref<'Movie' | 'Series'>('Movie')
const items = ref<LatestMediaItem[]>([])
const loading = ref(false)

const limit = 20
const placeholderPoster = 'https://via.placeholder.com/300x450?text=No+Poster'

const embyUrl = computed(() => userStore.embyUrl)

function getImageUrl(itemId: string) {
  if (!embyUrl.value) return placeholderPoster
  return `${embyUrl.value}/emby/Items/${itemId}/Images/Primary?maxHeight=400&quality=90`
}

const tabItems = computed(() => [
  { key: 'Movie' as const, label: '电影', icon: Film },
  { key: 'Series' as const, label: '剧集', icon: VideoPlay },
])

const skeletonItems = computed(() => Array.from({ length: 12 }))

async function ensureEmbyUrl() {
  if (embyUrl.value) return
  try {
    await userStore.fetchEmbyConfig()
  } catch {
    // 失败时不阻塞页面渲染，海报会回退到占位图
  }
}

async function fetchLatest() {
  loading.value = true
  try {
    await ensureEmbyUrl()
    const res = await getLatestMedia(activeType.value, limit)
    if (!res?.success) {
      items.value = []
      ElMessage.error('获取最近入库失败')
      return
    }
    items.value = res.data || []
  } catch {
    items.value = []
  } finally {
    loading.value = false
  }
}

watch(activeType, () => {
  fetchLatest()
})

onMounted(fetchLatest)
</script>

<template>
  <div class="space-y-6 animate-fade-in">
    <EmberPageHeaderCard
      title="媒体库"
      description="浏览 Emby 服务器最近入库的影视内容"
    >
      <template #actions>
        <div class="flex items-center gap-3">
          <EmberSegmentTabs
            v-model="activeType"
            :tabs="tabItems"
            :full-width="false"
          />

          <button
            type="button"
            aria-label="刷新最近入库"
            class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-700 transition-colors hover:bg-gray-50 cursor-pointer"
            :class="loading ? 'opacity-60 cursor-not-allowed' : ''"
            :disabled="loading"
            @click="fetchLatest"
          >
            <el-icon :size="18"><RefreshRight /></el-icon>
          </button>
        </div>
      </template>
    </EmberPageHeaderCard>

    <div v-if="loading" class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-6">
      <div v-for="(_x, idx) in skeletonItems" :key="idx" class="space-y-3">
        <div class="rounded-2xl bg-gray-100 aspect-[2/3] animate-pulse"></div>
        <div class="h-4 bg-gray-100 rounded-lg animate-pulse"></div>
        <div class="h-3 bg-gray-100 rounded-lg w-3/4 animate-pulse"></div>
      </div>
    </div>

    <EmberEmptyStateCard
      v-else-if="items.length === 0"
      :icon="Film"
      title="暂无最近入库内容"
      description="稍后刷新，或切换电影/剧集后再试。"
    />

    <div v-else class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-6">
      <div v-for="item in items" :key="item.id" class="group">
        <div class="rounded-2xl bg-white border border-gray-100 shadow-sm overflow-hidden hover:shadow-md hover:border-ember/30 transition-all">
          <div class="relative aspect-[2/3] bg-gray-100 overflow-hidden">
            <img
              :src="getImageUrl(item.id)"
              :alt="item.name"
              loading="lazy"
              class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-110"
              @error="(e: Event) => (((e.target as HTMLImageElement).src = placeholderPoster))"
            />
            <div class="absolute inset-0 bg-gradient-to-t from-black/70 via-black/10 to-transparent opacity-0 group-hover:opacity-100 transition-opacity"></div>
          </div>

          <div class="p-3 space-y-2">
            <div class="text-sm font-bold text-gray-900 line-clamp-2 min-h-[2.5rem]">
              {{ item.name }}
            </div>

            <div class="flex items-center justify-between gap-2 text-xs text-gray-500">
              <div class="flex items-center gap-2 min-w-0">
                <span v-if="item.productionYear" class="font-medium text-gray-600">
                  {{ item.productionYear }}
                </span>
                <span v-if="item.communityRating" class="text-amber-600 font-semibold">
                  ★ {{ item.communityRating.toFixed(1) }}
                </span>
              </div>

              <span
                v-if="item.type === 'Series' && item.childCount > 0"
                class="whitespace-nowrap text-[11px] px-2 py-0.5 rounded-full font-semibold bg-gray-900/5 text-gray-700 border border-gray-200"
              >
                +{{ item.childCount }}集
              </span>
            </div>

            <!-- 保持卡片高度一致：无分级时也预留一行 -->
            <div class="text-[11px] text-gray-500 min-h-[1rem]">
              <span v-if="item.officialRating">分级：{{ item.officialRating }}</span>
              <span v-else class="opacity-0" aria-hidden="true">分级：-</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
