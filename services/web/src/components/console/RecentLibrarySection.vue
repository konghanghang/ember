<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ArrowLeft, ArrowRight, Film, RefreshRight, VideoPlay } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import { getLatestMedia, getMediaPoster } from '@/api/console'
import { emberPosterPlaceholder } from '@/utils/posterPlaceholder'
import type { LatestMediaItem } from '@/types/api'

const props = withDefaults(defineProps<{
  limit?: number
}>(), {
  limit: 20
})

const activeType = ref<'Movie' | 'Series'>('Movie')
const items = ref<LatestMediaItem[]>([])
const loading = ref(false)
const hasLoadError = ref(false)
// emptyReason 区分"未配置 / 用户未绑 / 真错 / 单纯空"四种空态，
// 避免把 fresh-install 的合法初始化态误展示为"接口暂时不可用"。
type EmptyReason = 'unconfigured' | 'unbound' | null
const emptyReason = ref<EmptyReason>(null)
const scrollerRef = ref<HTMLElement | null>(null)

const placeholderPoster = emberPosterPlaceholder
const posterURLMap = ref<Record<string, string>>({})
const loadingPosterIDs = new Set<string>()
const objectURLs = new Set<string>()
let fetchLatestRequestToken = 0

const tabItems = computed(() => [
  { key: 'Movie' as const, label: '电影', icon: Film },
  { key: 'Series' as const, label: '剧集', icon: VideoPlay }
])

const skeletonItems = computed(() => Array.from({ length: props.limit }))

function getImageUrl(itemId: string) {
  return posterURLMap.value[itemId] || placeholderPoster
}

function cleanupPosterURLs(activeIDs: string[]) {
  const keep = new Set(activeIDs)

  for (const [id, url] of Object.entries(posterURLMap.value)) {
    if (keep.has(id) || url === placeholderPoster) {
      continue
    }

    URL.revokeObjectURL(url)
    objectURLs.delete(url)
    delete posterURLMap.value[id]
  }
}

async function preloadPosterURLs(nextItems: LatestMediaItem[], requestToken: number, mediaType: 'Movie' | 'Series') {
  for (const item of nextItems) {
    const id = item.id?.trim()
    if (!id || posterURLMap.value[id] || loadingPosterIDs.has(id)) {
      continue
    }

    loadingPosterIDs.add(id)
    try {
      const blob = await getMediaPoster(id, mediaType)
      if (requestToken !== fetchLatestRequestToken || mediaType !== activeType.value) {
        continue
      }
      if (blob && blob.size > 0) {
        const objectURL = URL.createObjectURL(blob)
        objectURLs.add(objectURL)
        posterURLMap.value[id] = objectURL
      } else {
        posterURLMap.value[id] = placeholderPoster
      }
    } catch {
      if (requestToken !== fetchLatestRequestToken || mediaType !== activeType.value) {
        continue
      }
      posterURLMap.value[id] = placeholderPoster
    } finally {
      loadingPosterIDs.delete(id)
    }
  }
}

async function fetchLatest() {
  const requestToken = ++fetchLatestRequestToken
  const mediaType = activeType.value
  loading.value = true
  hasLoadError.value = false
  emptyReason.value = null

  try {
    const res = await getLatestMedia(mediaType, props.limit)
    if (requestToken !== fetchLatestRequestToken || mediaType !== activeType.value) {
      return
    }
    if (!res?.success) {
      items.value = []
      hasLoadError.value = true
      cleanupPosterURLs([])
      return
    }

    if (res.configured === false) {
      items.value = []
      emptyReason.value = 'unconfigured'
      cleanupPosterURLs([])
      return
    }

    if (res.bound === false) {
      items.value = []
      emptyReason.value = 'unbound'
      cleanupPosterURLs([])
      return
    }

    items.value = res.data || []
    cleanupPosterURLs(items.value.map((item) => item.id))
    void preloadPosterURLs(items.value, requestToken, mediaType)
  } catch {
    if (requestToken !== fetchLatestRequestToken || mediaType !== activeType.value) {
      return
    }
    items.value = []
    hasLoadError.value = true
    cleanupPosterURLs([])
  } finally {
    if (requestToken === fetchLatestRequestToken) {
      loading.value = false
    }
  }
}

function scrollByDirection(direction: 'left' | 'right') {
  if (!scrollerRef.value) return

  const offset = Math.max(scrollerRef.value.clientWidth * 0.85, 320)
  scrollerRef.value.scrollBy({
    left: direction === 'left' ? -offset : offset,
    behavior: 'smooth'
  })
}

watch(activeType, () => {
  fetchLatest()
})

onMounted(() => {
  fetchLatest()
})

onBeforeUnmount(() => {
  for (const url of objectURLs) {
    URL.revokeObjectURL(url)
  }
  objectURLs.clear()
})
</script>

<template>
  <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-5 lg:flex-row lg:items-center lg:justify-between">
      <h2 class="text-lg font-semibold text-gray-900">最近入库</h2>

      <div class="flex flex-wrap items-center gap-3">
        <EmberSegmentTabs
          v-model="activeType"
          :tabs="tabItems"
          :full-width="false"
          aria-label="最近入库媒体类型切换"
        />

        <button
          type="button"
          aria-label="刷新最近入库"
          class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-700 transition-colors hover:bg-gray-50 cursor-pointer"
          :class="loading ? 'cursor-not-allowed opacity-60' : ''"
          :disabled="loading"
          @click="fetchLatest"
        >
          <el-icon :size="18"><RefreshRight /></el-icon>
        </button>

        <div class="hidden items-center gap-2 lg:flex">
          <button
            type="button"
            aria-label="向左滑动最近入库"
            class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-700 transition-colors hover:bg-gray-50 cursor-pointer"
            @click="scrollByDirection('left')"
          >
            <el-icon :size="18"><ArrowLeft /></el-icon>
          </button>
          <button
            type="button"
            aria-label="向右滑动最近入库"
            class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-700 transition-colors hover:bg-gray-50 cursor-pointer"
            @click="scrollByDirection('right')"
          >
            <el-icon :size="18"><ArrowRight /></el-icon>
          </button>
        </div>
      </div>
    </div>

    <div class="p-6">
      <div v-if="loading" class="overflow-x-auto pb-2 recent-library-scroller">
        <div class="flex min-w-max gap-4">
          <div v-for="(_item, idx) in skeletonItems" :key="idx" class="w-[9.5rem] shrink-0 space-y-3 sm:w-[10.5rem] lg:w-[11rem]">
          <div class="aspect-[2/3] animate-pulse rounded-2xl bg-gray-100"></div>
          <div class="h-4 animate-pulse rounded-lg bg-gray-100"></div>
          <div class="h-3 w-3/4 animate-pulse rounded-lg bg-gray-100"></div>
          </div>
        </div>
      </div>

      <EmberEmptyStateCard
        v-else-if="emptyReason === 'unconfigured'"
        :icon="Film"
        tone="warning"
        title="系统尚未配置 Emby 服务器"
        description="管理员完成 Emby 连接配置后，最近入库会自动出现在这里。"
      />

      <EmberEmptyStateCard
        v-else-if="emptyReason === 'unbound'"
        :icon="Film"
        title="等待管理员开通 Emby 账号"
        description="账号尚未关联到 Emby 媒体库，建库后这里会展示最新入库的影片。"
      />

      <EmberEmptyStateCard
        v-else-if="hasLoadError"
        :icon="Film"
        title="最近入库暂时不可用"
        description="接口获取失败时只降级当前区块，稍后可以手动刷新重试。"
      />

      <EmberEmptyStateCard
        v-else-if="items.length === 0"
        :icon="Film"
        title="暂无最近入库内容"
        description="当前分类还没有可展示的新内容，可以切换电影或剧集后再看。"
      />

      <div
        v-else
        ref="scrollerRef"
        class="overflow-x-auto pb-2 recent-library-scroller"
      >
        <div class="flex min-w-max gap-4">
        <article
          v-for="item in items"
          :key="item.id"
          class="group w-[9.5rem] shrink-0 overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm transition-all hover:border-ember/30 hover:shadow-md snap-start sm:w-[10.5rem] lg:w-[11rem]"
        >
          <div class="relative aspect-[2/3] overflow-hidden bg-gray-100">
            <img
              :src="getImageUrl(item.id)"
              :alt="item.name"
              loading="lazy"
              class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
              @error="(event: Event) => (((event.target as HTMLImageElement).src = placeholderPoster))"
            />
            <div class="absolute inset-0 bg-gradient-to-t from-black via-black/55 to-transparent opacity-90"></div>

            <div class="absolute left-3 top-3 text-white/80">
              <el-icon v-if="item.type === 'Movie'" :size="16"><Film /></el-icon>
              <el-icon v-else :size="16"><VideoPlay /></el-icon>
            </div>

            <div
              v-if="item.type === 'Series' && item.childCount > 0"
              class="absolute right-3 top-3 rounded-full bg-white/15 px-2 py-1 text-[10px] font-semibold text-white backdrop-blur-sm"
            >
              +{{ item.childCount }} 集
            </div>

            <div class="absolute inset-x-0 bottom-0 p-3 text-white">
              <h3 class="line-clamp-2 text-sm font-semibold leading-5 drop-shadow-sm" :title="item.name">
                {{ item.name }}
              </h3>

              <div class="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-white/75">
                <span v-if="item.productionYear">{{ item.productionYear }}</span>
                <span v-if="item.communityRating" class="font-semibold text-amber-200">
                  ★ {{ item.communityRating.toFixed(1) }}
                </span>
                <span v-if="item.officialRating" class="rounded-full bg-white/15 px-2 py-1 backdrop-blur-sm">
                  {{ item.officialRating }}
                </span>
              </div>
            </div>
          </div>
        </article>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.recent-library-scroller {
  scroll-snap-type: x proximity;
  scrollbar-width: thin;
}
</style>
