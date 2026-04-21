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
            aria-label="媒体类型切换"
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

    <div v-if="loading" class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-5 gap-6">
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

    <div v-else class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-5 gap-6">
      <div v-for="item in items" :key="item.id" class="group">
        <article class="overflow-hidden rounded-2xl bg-white border border-gray-100 shadow-sm transition-all hover:shadow-md hover:border-ember/30">
          <div class="relative aspect-[2/3] bg-gray-100 overflow-hidden">
            <img
              :src="getImageUrl(item.id)"
              :alt="item.name"
              loading="lazy"
              class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-110"
              @error="(e: Event) => (((e.target as HTMLImageElement).src = placeholderPoster))"
            />
            <div class="absolute inset-0 bg-gradient-to-t from-black via-black/70 via-black/15 to-transparent opacity-90"></div>

            <div class="absolute left-2 top-2 text-white/80">
              <el-icon v-if="item.type === 'Movie'" :size="16"><Film /></el-icon>
              <el-icon v-else :size="16"><VideoPlay /></el-icon>
            </div>

            <div
              v-if="item.type === 'Series' && item.childCount > 0"
              class="absolute right-2 top-2"
            >
              <span class="rounded px-2 py-0.5 text-[10px] font-bold text-white shadow-sm backdrop-blur-md bg-white/15">
                +{{ item.childCount }}集
              </span>
            </div>

            <div class="absolute inset-x-0 bottom-0 p-3 text-white">
              <h3 class="line-clamp-2 text-sm font-bold leading-5 drop-shadow-sm" :title="item.name">
                {{ item.name }}
              </h3>

              <div class="mt-1 flex items-center gap-2 text-[11px] text-white/70">
                <span v-if="item.productionYear">{{ item.productionYear }}</span>
                <span v-if="item.communityRating" class="font-semibold text-amber-200">
                  ★ {{ item.communityRating.toFixed(1) }}
                </span>
              </div>

              <div class="mt-2 flex flex-wrap gap-1.5">
                <span
                  v-if="item.officialRating"
                  class="inline-flex items-center rounded-full bg-white/15 px-2 py-1 text-[10px] font-medium text-white/90 backdrop-blur-sm"
                >
                  分级 {{ item.officialRating }}
                </span>
                <span
                  v-if="item.type === 'Movie'"
                  class="inline-flex items-center rounded-full bg-white/15 px-2 py-1 text-[10px] font-medium text-white/90 backdrop-blur-sm"
                >
                  电影
                </span>
                <span
                  v-else
                  class="inline-flex items-center rounded-full bg-white/15 px-2 py-1 text-[10px] font-medium text-white/90 backdrop-blur-sm"
                >
                  剧集
                </span>
              </div>
            </div>
          </div>
        </article>
      </div>
    </div>
  </div>
</template>
