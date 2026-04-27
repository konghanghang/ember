<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, Film, VideoPlay, Check, RefreshRight } from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import { getTmdbTVSeasons, searchTmdb } from '@/api/user'
import { checkExistingSubscription, createSubscription } from '@/api/console'
import { emberPosterPlaceholder } from '@/utils/posterPlaceholder'
import type {
  CreateSubscriptionRequest,
  MediaType,
  SubscriptionExistingSummary,
  TmdbSearchItem
} from '@/types/api'

const router = useRouter()
const searchQuery = ref('')
const searchType = ref<MediaType>('MOVIE')
const loading = ref(false)
const results = ref<TmdbSearchItem[]>([])
const hasSearched = ref(false)
const searchFailed = ref(false)

const selectedItem = ref<TmdbSearchItem | null>(null)
const overviewExpanded = ref(false)
const subscriptionForm = ref({
  season: null as number | null,
  note: ''
})
const seasonOptions = ref<Array<{ label: string; value: number }>>([])
const seasonOptionsLoading = ref(false)
const seasonOptionsError = ref('')
const submitting = ref(false)
const checkingExisting = ref(false)
const showConfirmDialog = ref(false)
let seasonRequestToken = 0
let searchRequestToken = 0
let timeout: ReturnType<typeof setTimeout>

const typeTabs = computed(() => [
  { key: 'MOVIE', label: '电影', icon: Film },
  { key: 'TV', label: '剧集', icon: VideoPlay }
])
const overviewText = computed(() => selectedItem.value?.overview?.trim() || '暂无简介')
const hasExpandableOverview = computed(() => {
  const overview = selectedItem.value?.overview?.trim()
  return Boolean(overview && overview.length > 120)
})

const handleSearch = async () => {
  const requestToken = ++searchRequestToken

  if (!searchQuery.value.trim()) {
    if (requestToken === searchRequestToken) {
      results.value = []
      hasSearched.value = false
      searchFailed.value = false
      loading.value = false
    }
    return
  }

  if (requestToken === searchRequestToken) {
    loading.value = true
    hasSearched.value = true
  }
  try {
    const type = searchType.value === 'MOVIE' ? 'movie' : 'tv'
    const res = await searchTmdb(searchQuery.value, type, { silent: true })
    if (requestToken !== searchRequestToken) return
    results.value = res.results || []
    searchFailed.value = false
  } catch {
    if (requestToken !== searchRequestToken) return
    results.value = []
    searchFailed.value = true
  } finally {
    if (requestToken === searchRequestToken) {
      loading.value = false
    }
  }
}

watch(searchQuery, () => {
  clearTimeout(timeout)
  timeout = setTimeout(handleSearch, 500)
})

watch(searchType, () => {
  if (searchQuery.value) handleSearch()
})

watch(showConfirmDialog, (visible) => {
  if (visible) return

  seasonRequestToken++
  overviewExpanded.value = false
  selectedItem.value = null
  resetSeasonOptions()
})

const resetSeasonOptions = () => {
  subscriptionForm.value.season = null
  seasonOptions.value = []
  seasonOptionsError.value = ''
  seasonOptionsLoading.value = false
}

const loadSeasonOptions = async (item: TmdbSearchItem) => {
  const requestToken = ++seasonRequestToken
  seasonOptionsLoading.value = true
  seasonOptionsError.value = ''

  try {
    const res = await getTmdbTVSeasons(item.id)
    if (requestToken != seasonRequestToken) return

    const seasons = Array.isArray(res.data?.seasons) ? res.data.seasons : []
    seasonOptions.value = seasons.map((season) => ({
      label: `第 ${season} 季`,
      value: season
    }))
    if (seasonOptions.value.length === 0) {
      seasonOptionsError.value = 'TMDB 没有返回可选季列表，当前无法按季提交。'
      return
    }
    if (subscriptionForm.value.season == null) {
      subscriptionForm.value.season = seasons.includes(1) ? 1 : seasons[0]
    }
  } catch {
    if (requestToken != seasonRequestToken) return
    subscriptionForm.value.season = null
    seasonOptions.value = []
    seasonOptionsError.value = '季列表加载失败，可重试后选择具体季数。'
  } finally {
    if (requestToken == seasonRequestToken) {
      seasonOptionsLoading.value = false
    }
  }
}

const selectItem = async (item: TmdbSearchItem) => {
  selectedItem.value = item
  overviewExpanded.value = false
  subscriptionForm.value.season = 0
  subscriptionForm.value.note = ''
  resetSeasonOptions()
  showConfirmDialog.value = true

  if (searchType.value === 'TV') {
    await loadSeasonOptions(item)
  }
}

const buildSubscriptionPayload = (confirmExisting = false): CreateSubscriptionRequest | null => {
  if (!selectedItem.value) return null

  return {
    type: searchType.value,
    name: selectedItem.value.title,
    tmdbId: selectedItem.value.id.toString(),
    season: searchType.value === 'TV' ? (subscriptionForm.value.season ?? undefined) : 0,
    posterPath: selectedItem.value.posterPath,
    note: subscriptionForm.value.note,
    confirmExisting
  }
}

const formatExistingSummary = (summary?: SubscriptionExistingSummary) => {
  if (!summary) {
    return '库内已存在相关资源，确认后仍可继续提交。'
  }

  const seasonText = summary.availableSeasons?.length
    ? `已入库季：${summary.availableSeasons.join('、')}`
    : ''
  const episodeText = summary.episodeCount ? `已入库 ${summary.episodeCount} 集` : ''

  return [summary.message, seasonText, episodeText].filter(Boolean).join('\n')
}

const getExistingConfirmationTitle = (summary?: SubscriptionExistingSummary) => {
  if (summary?.detectionFailed) {
    return '库内检测失败，是否仍继续提交'
  }
  return '检测到库内已存在相关资源'
}

const submitSubscription = async (confirmExisting = false) => {
  const payload = buildSubscriptionPayload(confirmExisting)
  if (!payload) return

  await createSubscription(payload)
  ElMessage.success('订阅提交成功')
  showConfirmDialog.value = false
  router.push('/console/subscriptions')
}

const requestExistingConfirmation = async (summary?: SubscriptionExistingSummary) => {
  try {
    await ElMessageBox.confirm('', getExistingConfirmationTitle(summary), {
      type: 'warning',
      confirmButtonText: '仍然提交',
      cancelButtonText: '取消',
      message: () => h('div', { class: 'whitespace-pre-line' }, formatExistingSummary(summary))
    })
  } catch {
    return
  }

  await submitSubscription(true)
}

const confirmSubscription = async () => {
  const payload = buildSubscriptionPayload()
  if (!payload) return

  submitting.value = true
  try {
    checkingExisting.value = true
    const existing = await checkExistingSubscription({
      type: payload.type,
      tmdbId: payload.tmdbId,
      season: payload.season
    })
    checkingExisting.value = false

    if (existing.existsInLibrary || existing.detectionFailed) {
      await requestExistingConfirmation(existing.existingSummary)
      return
    }

    await submitSubscription(false)
  } catch (error: any) {
    const responseData = error?.response?.data
    if (responseData?.confirmationRequired) {
      await requestExistingConfirmation(responseData.existingSummary)
      return
    }
  } finally {
    checkingExisting.value = false
    submitting.value = false
  }
}

const getImageUrl = (path?: string) => {
  return path ? `https://image.tmdb.org/t/p/w500${path}` : emberPosterPlaceholder
}

const retryLoadSeasonOptions = async () => {
  if (!selectedItem.value || searchType.value !== 'TV') return
  await loadSeasonOptions(selectedItem.value)
}

const isConfirmDisabled = () => {
  if (submitting.value || checkingExisting.value) return true
  if (searchType.value !== 'TV') return false
  return seasonOptionsLoading.value || subscriptionForm.value.season == null
}
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-8">
    <EmberPageHeaderCard title="添加新订阅" description="搜索你想观看的电影或剧集，并提交订阅请求。">
      <div class="mx-auto mt-6 flex max-w-3xl flex-col gap-4">
        <div class="flex justify-center">
          <EmberSegmentTabs v-model="searchType" :tabs="typeTabs" :full-width="false" aria-label="订阅类型切换" />
        </div>

        <div class="relative">
          <EmberSearchInput
            v-model="searchQuery"
            label="影视搜索"
            aria-label="搜索影视作品"
            placeholder="输入名称搜索..."
            :icon="Search"
            type="text"
            inputmode="search"
            autocomplete="off"
          />
          <div v-if="loading" class="pointer-events-none absolute bottom-3 right-4 flex items-center">
            <div class="h-5 w-5 animate-spin rounded-full border-2 border-gray-300 border-t-ember"></div>
          </div>
        </div>
      </div>
    </EmberPageHeaderCard>

    <div v-if="results.length > 0" class="grid animate-fade-in-up grid-cols-2 gap-6 md:grid-cols-4 lg:grid-cols-5">
      <button
        v-for="item in results"
        :key="item.id"
        type="button"
        class="group relative overflow-hidden rounded-xl border border-gray-100 bg-white text-left shadow-sm transition-all duration-300 hover:border-ember/30 hover:shadow-xl focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-ember/10"
        :aria-label="`选择 ${item.title}`"
        @click="selectItem(item)"
      >
        <div class="relative aspect-[2/3] overflow-hidden bg-gray-100">
          <img
            :src="getImageUrl(item.posterPath)"
            :alt="`${item.title} 海报`"
            class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-110"
            loading="lazy"
          />
          <div class="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity duration-300 group-hover:opacity-100 group-focus-visible:opacity-100">
            <div class="relative flex h-14 w-14 translate-y-4 items-center justify-center rounded-full bg-ember shadow-lg transition-transform duration-300 group-hover:translate-y-0 group-focus-visible:translate-y-0">
              <span aria-hidden="true" class="absolute h-0.5 w-5 rounded-full bg-white"></span>
              <span aria-hidden="true" class="absolute h-5 w-0.5 rounded-full bg-white"></span>
            </div>
          </div>
        </div>

        <div class="p-4">
          <h3 class="line-clamp-1 font-bold text-gray-900 transition-colors group-hover:text-ember" :title="item.title">{{ item.title }}</h3>
          <div class="mt-2 flex items-center justify-between text-xs text-gray-500">
            <span>{{ item.releaseDate ? new Date(item.releaseDate).getFullYear() : '未知年份' }}</span>
            <span class="rounded bg-gray-100 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide">
              {{ searchType === 'MOVIE' ? 'Movie' : 'TV Series' }}
            </span>
          </div>
        </div>
      </button>
    </div>

    <EmberEmptyStateCard
      v-else-if="searchFailed && hasSearched && !loading"
      :icon="RefreshRight"
      tone="danger"
      title="搜索失败"
      description="当前无法从 TMDB 获取搜索结果，请稍后重试。"
    >
      <template #actions>
        <button
          type="button"
          @click="handleSearch"
          class="rounded-xl border border-red-200 bg-white px-4 py-2 text-sm font-semibold text-red-700 transition-colors hover:bg-red-50"
        >
          重新搜索
        </button>
      </template>
    </EmberEmptyStateCard>

    <EmberEmptyStateCard
      v-else-if="hasSearched && !loading"
      :icon="Search"
      title="未找到相关结果"
      description="请尝试其他关键词，或切换电影 / 剧集后重新搜索。"
    />

    <EmberEmptyStateCard
      v-else-if="!hasSearched"
      :icon="Film"
      title="开始搜索你喜欢的影视作品"
      description="支持中文和英文搜索。"
    />

    <EmberFormDialog
      v-model="showConfirmDialog"
      title="确认订阅"
      width="min(780px, calc(100vw - 2rem))"
      :show-close="false"
    >
      <div v-if="selectedItem" class="flex flex-col gap-5 sm:flex-row sm:items-start sm:gap-6">
        <div class="mx-auto w-44 flex-shrink-0 sm:mx-0 sm:w-48 md:w-52">
          <img :src="getImageUrl(selectedItem.posterPath)" :alt="`${selectedItem.title} 海报`" class="block w-full rounded-xl bg-gray-100 shadow-md" />
        </div>
        <div class="min-w-0 flex-1 space-y-4">
          <div>
            <h3 class="text-xl font-bold leading-tight text-gray-900 md:text-[1.75rem]">{{ selectedItem.title }}</h3>
            <p class="mt-1 text-sm text-gray-500">
              {{ selectedItem.releaseDate }} · {{ searchType === 'MOVIE' ? '电影' : '剧集' }}
            </p>
          </div>

          <div class="rounded-xl border border-gray-100 bg-gray-50 p-4">
            <div
              class="text-sm leading-7 text-gray-600"
              :class="!overviewExpanded && hasExpandableOverview ? 'line-clamp-5' : ''"
            >
              {{ overviewText }}
            </div>
            <div v-if="hasExpandableOverview" class="mt-3 flex justify-end">
              <button
                type="button"
                class="cursor-pointer text-sm font-semibold text-ember transition-colors hover:text-ember/80"
                @click="overviewExpanded = !overviewExpanded"
              >
                {{ overviewExpanded ? '收起简介' : '展开简介' }}
              </button>
            </div>
          </div>

          <div>
            <div v-if="searchType === 'TV'" class="mb-4">
              <label class="mb-1.5 block text-xs font-bold uppercase tracking-wider text-gray-500">季数</label>
              <div class="space-y-2">
                <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                  <el-select
                    v-model="subscriptionForm.season"
                    class="form-select !w-full sm:!w-52"
                    :disabled="seasonOptionsLoading"
                    :loading="seasonOptionsLoading"
                    placeholder="选择季数"
                  >
                    <el-option
                      v-for="option in seasonOptions"
                      :key="option.value"
                      :label="option.label"
                      :value="option.value"
                    />
                  </el-select>
                  <button
                    v-if="seasonOptionsError"
                    type="button"
                    class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-semibold text-gray-600 transition-colors hover:bg-gray-50"
                    @click="retryLoadSeasonOptions"
                  >
                    <span class="inline-flex items-center gap-1.5">
                      <el-icon><RefreshRight /></el-icon>
                      重试
                    </span>
                  </button>
                </div>
                <p v-if="seasonOptionsLoading" class="text-xs text-gray-500">正在读取 TMDB 季列表...</p>
                <p v-else-if="seasonOptionsError" class="text-xs text-amber-600">{{ seasonOptionsError }}</p>
              </div>
            </div>

            <label class="mb-1.5 block text-xs font-bold uppercase tracking-wider text-gray-500">备注信息 (可选)</label>
            <el-input
              v-model="subscriptionForm.note"
              type="textarea"
              :rows="3"
              placeholder="例如：希望能尽快下载，或者指定版本"
              class="input-ember"
            />
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex flex-col-reverse gap-3 border-t border-gray-100 pt-4 sm:flex-row sm:justify-end">
          <button
            type="button"
            @click="showConfirmDialog = false"
            class="rounded-lg px-4 py-2 font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            type="button"
            @click="confirmSubscription"
            :disabled="isConfirmDisabled()"
            class="btn-ember inline-flex items-center gap-2 rounded-lg px-6 py-2 font-bold shadow-md transition-colors hover:shadow-lg disabled:cursor-not-allowed disabled:opacity-70"
          >
            <span v-if="submitting || checkingExisting" class="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
            <el-icon v-else><Check /></el-icon>
            {{ submitting ? '提交中...' : checkingExisting ? '检测中...' : '确认订阅' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>

<style scoped>
.animate-fade-in-up {
  animation: fadeInUp 0.5s ease-out forwards;
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
