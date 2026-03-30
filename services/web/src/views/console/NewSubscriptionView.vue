<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search, Film, VideoPlay, Plus, Check, RefreshRight } from '@element-plus/icons-vue'
import { getTmdbTVSeasons, searchTmdb } from '@/api/user'
import { createSubscription } from '@/api/console'
import type { CreateSubscriptionRequest, MediaType, TmdbSearchItem } from '@/types/api'

const router = useRouter()
const searchQuery = ref('')
const searchType = ref<MediaType>('MOVIE')
const loading = ref(false)
const results = ref<TmdbSearchItem[]>([])
const hasSearched = ref(false)

// Selection State
const selectedItem = ref<TmdbSearchItem | null>(null)
const subscriptionForm = ref({
  season: null as number | null,
  note: ''
})
const seasonOptions = ref<Array<{ label: string; value: number }>>([])
const seasonOptionsLoading = ref(false)
const seasonOptionsError = ref('')
const submitting = ref(false)
const showConfirmDialog = ref(false)
let seasonRequestToken = 0

// Debounce search
let timeout: ReturnType<typeof setTimeout>

const handleSearch = async () => {
  if (!searchQuery.value.trim()) {
    results.value = []
    hasSearched.value = false
    return
  }

  loading.value = true
  hasSearched.value = true
  try {
    const type = searchType.value === 'MOVIE' ? 'movie' : 'tv'
    const res = await searchTmdb(searchQuery.value, type)
    results.value = res.results || []
  } catch (error) {
    ElMessage.error('搜索失败，请稍后重试')
    results.value = []
  } finally {
    loading.value = false
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
  } catch (error) {
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
  subscriptionForm.value.season = 0
  subscriptionForm.value.note = ''
  resetSeasonOptions()
  showConfirmDialog.value = true

  if (searchType.value === 'TV') {
    await loadSeasonOptions(item)
  }
}

const confirmSubscription = async () => {
  if (!selectedItem.value) return

  submitting.value = true
  try {
    const payload: CreateSubscriptionRequest = {
      type: searchType.value,
      name: selectedItem.value.title,
      tmdbId: selectedItem.value.id.toString(),
      season: searchType.value === 'TV' ? (subscriptionForm.value.season ?? undefined) : 0,
      posterPath: selectedItem.value.posterPath,
      note: subscriptionForm.value.note
    }

    await createSubscription(payload)
    ElMessage.success('订阅提交成功')
    showConfirmDialog.value = false
    router.push('/console/subscriptions')
  } catch (error: any) {
    // Error handled by interceptor
  } finally {
    submitting.value = false
  }
}

const getImageUrl = (path?: string) => {
  return path ? `https://image.tmdb.org/t/p/w500${path}` : 'https://via.placeholder.com/500x750?text=No+Poster'
}

const retryLoadSeasonOptions = async () => {
  if (!selectedItem.value || searchType.value !== 'TV') return
  await loadSeasonOptions(selectedItem.value)
}

const isConfirmDisabled = () => {
  if (submitting.value) return true
  if (searchType.value !== 'TV') return false
  return seasonOptionsLoading.value || subscriptionForm.value.season == null
}
</script>

<template>
  <div class="max-w-7xl mx-auto space-y-8">
    <!-- Header & Search Area -->
    <div class="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 md:p-8">
      <div class="max-w-3xl mx-auto text-center space-y-6">
        <h1 class="text-2xl md:text-3xl font-bold text-gray-900">添加新订阅</h1>
        <p class="text-gray-500">搜索您想要观看的电影或剧集，提交订阅请求。</p>

        <!-- Search Controls -->
        <div class="flex flex-col gap-4">
          <!-- Type Toggle -->
          <div class="flex justify-center">
            <div class="inline-flex bg-gray-100 p-1 rounded-xl">
              <button 
                @click="searchType = 'MOVIE'"
                class="px-6 py-2 rounded-lg text-sm font-bold transition-all duration-200 flex items-center gap-2"
                :class="searchType === 'MOVIE' ? 'bg-white text-ember shadow-sm' : 'text-gray-500 hover:text-gray-700'"
              >
                <el-icon><Film /></el-icon>
                电影
              </button>
              <button 
                @click="searchType = 'TV'"
                class="px-6 py-2 rounded-lg text-sm font-bold transition-all duration-200 flex items-center gap-2"
                :class="searchType === 'TV' ? 'bg-white text-ember shadow-sm' : 'text-gray-500 hover:text-gray-700'"
              >
                <el-icon><VideoPlay /></el-icon>
                剧集
              </button>
            </div>
          </div>

          <!-- Search Input -->
          <div class="relative group">
            <div class="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
              <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors" :size="20"><Search /></el-icon>
            </div>
            <input 
              v-model="searchQuery"
              type="text" 
              aria-label="搜索影视作品"
              class="w-full pl-12 pr-4 py-4 bg-gray-50 border border-gray-200 rounded-xl text-lg outline-none focus:bg-white focus:border-ember focus:ring-4 focus:ring-ember/10 transition-all placeholder-gray-400"
              placeholder="输入名称搜索..."
              autofocus
            />
            <div v-if="loading" class="absolute inset-y-0 right-0 pr-4 flex items-center">
              <div class="animate-spin rounded-full h-5 w-5 border-b-2 border-ember"></div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Results Grid -->
    <div v-if="results.length > 0" class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-6 animate-fade-in-up">
      <div 
        v-for="item in results" 
        :key="item.id"
        class="group relative bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden hover:shadow-xl hover:border-ember/30 transition-all duration-300 cursor-pointer"
        @click="selectItem(item)"
      >
        <!-- Poster -->
        <div class="aspect-[2/3] relative overflow-hidden bg-gray-100">
          <img 
            :src="getImageUrl(item.posterPath)" 
            class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-110"
            loading="lazy"
          />
          <!-- Overlay on hover -->
          <div class="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center">
            <div class="bg-ember text-white rounded-full p-3 transform translate-y-4 group-hover:translate-y-0 transition-transform duration-300 shadow-lg">
              <el-icon :size="24"><Plus /></el-icon>
            </div>
          </div>
        </div>

        <!-- Info -->
        <div class="p-4">
          <h3 class="font-bold text-gray-900 line-clamp-1 group-hover:text-ember transition-colors" :title="item.title">{{ item.title }}</h3>
          <div class="flex items-center justify-between mt-2 text-xs text-gray-500">
            <span>{{ item.releaseDate ? new Date(item.releaseDate).getFullYear() : '未知年份' }}</span>
            <span class="px-2 py-0.5 bg-gray-100 rounded text-[10px] font-medium uppercase tracking-wide">
              {{ searchType === 'MOVIE' ? 'Movie' : 'TV Series' }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="hasSearched && !loading" class="text-center py-20 text-gray-400 bg-white rounded-2xl border border-dashed border-gray-200">
      <el-icon :size="48" class="mb-4 text-gray-300"><Search /></el-icon>
      <p>未找到相关结果，请尝试其他关键词</p>
    </div>

    <!-- Initial State -->
    <div v-else-if="!hasSearched" class="text-center py-20 text-gray-400">
      <div class="inline-flex justify-center items-center w-20 h-20 bg-gray-50 rounded-full mb-6 text-gray-300">
        <el-icon :size="40"><Film /></el-icon>
      </div>
      <p class="text-lg font-medium text-gray-500">开始搜索您喜欢的影视作品</p>
      <p class="text-sm mt-2">支持中文、英文搜索</p>
    </div>

    <!-- Confirmation Dialog -->
    <el-dialog
      v-model="showConfirmDialog"
      title="确认订阅"
      width="min(780px, calc(100vw - 2rem))"
      align-center
      append-to-body
      class="rounded-2xl overflow-hidden"
      :show-close="false"
    >
      <div v-if="selectedItem" class="flex flex-col gap-5 sm:flex-row sm:items-start sm:gap-6">
        <div class="mx-auto w-44 flex-shrink-0 sm:mx-0 sm:w-48 md:w-52">
          <img :src="getImageUrl(selectedItem.posterPath)" class="block w-full rounded-xl bg-gray-100 shadow-md" />
        </div>
        <div class="min-w-0 flex-1 space-y-4">
          <div>
            <h3 class="text-xl font-bold leading-tight text-gray-900 md:text-[1.75rem]">{{ selectedItem.title }}</h3>
            <p class="text-sm text-gray-500 mt-1">
              {{ selectedItem.releaseDate }} · {{ searchType === 'MOVIE' ? '电影' : '剧集' }}
            </p>
          </div>
          
          <div class="rounded-xl border border-gray-100 bg-gray-50 p-3 text-sm leading-6 text-gray-600 sm:line-clamp-4">
            {{ selectedItem.overview || '暂无简介' }}
          </div>

          <div>
            <div v-if="searchType === 'TV'" class="mb-4">
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider mb-1.5 block">季数</label>
              <div class="space-y-2">
                <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                  <el-select
                    v-model="subscriptionForm.season"
                    class="season-select !w-full sm:!w-52"
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
                    class="px-3 py-2 text-xs font-semibold text-gray-600 bg-white border border-gray-200 rounded-xl hover:bg-gray-50 transition-colors"
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

            <label class="text-xs font-bold text-gray-500 uppercase tracking-wider mb-1.5 block">备注信息 (可选)</label>
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
            @click="showConfirmDialog = false" 
            class="px-4 py-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors font-medium"
          >
            取消
          </button>
          <button 
            @click="confirmSubscription" 
            :disabled="isConfirmDisabled()"
            class="px-6 py-2 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg flex items-center gap-2 disabled:opacity-70 disabled:cursor-not-allowed"
          >
            <span v-if="submitting" class="animate-spin w-4 h-4 border-2 border-white/30 border-t-white rounded-full"></span>
            <el-icon v-else><Check /></el-icon>
            {{ submitting ? '提交中...' : '确认订阅' }}
          </button>
        </div>
      </template>
    </el-dialog>
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

:deep(.el-dialog) {
  border-radius: 16px;
  overflow: hidden;
}

:deep(.el-dialog__header) {
  margin-right: 0;
  border-bottom: 1px solid #f3f4f6;
  padding: 20px 24px;
}

:deep(.el-dialog__body) {
  padding: 24px;
}

:deep(.el-dialog__footer) {
  padding: 0 24px 24px;
}

:deep(.season-select .el-select__wrapper) {
  min-height: 42px;
  border-radius: 0.75rem;
  background-color: #f9fafb;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.season-select .el-select__wrapper:hover) {
  background-color: #ffffff;
}

:deep(.season-select .el-select__wrapper.is-focused),
:deep(.season-select .el-select__wrapper.is-focus),
:deep(.season-select.is-focus .el-select__wrapper) {
  background-color: #ffffff;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.season-select .el-select__selection) {
  min-height: 0;
}

:deep(.el-textarea__inner) {
  border-radius: 8px;
  background-color: #f9fafb;
  border-color: #e5e7eb;
  transition: all 0.2s;
}

:deep(.el-textarea__inner:focus) {
  background-color: white;
  border-color: var(--ember-red);
  box-shadow: 0 0 0 3px rgba(229, 9, 20, 0.1);
}
</style>
