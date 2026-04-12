<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getUserPlaybackProfile } from '@/api/admin'
import PlaybackProfileContent from '@/components/profile/PlaybackProfileContent.vue'
import type {
  PlaybackProfileRange,
  UserPlaybackProfile
} from '@/types/api'

const route = useRoute()
const router = useRouter()
const MAX_CUSTOM_RANGE_DAYS = 92

const loading = ref(false)
const profile = ref<UserPlaybackProfile | null>(null)
const customDateRange = ref<[string, string] | null>(null)
const rangeAnchorDate = ref<Date | null>(null)

const rangeOptions: Array<{ label: string; value: PlaybackProfileRange }> = [
  { label: '当天', value: 'today' },
  { label: '近 7 天', value: '7d' },
  { label: '近 30 天', value: '30d' },
  { label: '近 90 天', value: '90d' }
]

const rangeLabelMap: Record<PlaybackProfileRange, string> = {
  today: '当天',
  '7d': '近 7 天',
  '30d': '近 30 天',
  '90d': '近 90 天',
  all: '全部历史',
  custom: '自定义范围'
}

const normalizeRange = (value: unknown): PlaybackProfileRange => {
  const raw = String(value ?? '').trim()
  if (raw === 'today' || raw === '7d' || raw === '30d' || raw === '90d' || raw === 'all' || raw === 'custom') {
    return raw
  }
  return 'today'
}

const selectedRange = computed<PlaybackProfileRange>(() => normalizeRange(route.query.range))
const isCustomRange = computed(() => selectedRange.value === 'custom')
const selectedRangeLabel = computed(() => {
  if (isCustomRange.value && customDateRange.value) {
    return `${customDateRange.value[0]} ~ ${customDateRange.value[1]}`
  }
  return rangeLabelMap[selectedRange.value]
})
const userId = computed(() => String(route.params.id ?? '').trim())
const profileTitle = computed(() => profile.value?.username || '用户画像')

const buildDateString = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  const second = String(date.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}:${second}`
}

const buildPresetDateRange = (range: PlaybackProfileRange): [string, string] | null => {
  if (range === 'all' || range === 'custom') return null

  const end = new Date()
  const start = new Date()
  if (range === 'today') {
    start.setHours(0, 0, 0, 0)
  }
  if (range === '7d') start.setDate(start.getDate() - 7)
  if (range === '30d') start.setDate(start.getDate() - 30)
  if (range === '90d') start.setDate(start.getDate() - 90)

  return [buildDateString(start), buildDateString(end)]
}

const syncCustomRangeFromRoute = () => {
  const startDate = String(route.query.startDate ?? '').trim()
  const endDate = String(route.query.endDate ?? '').trim()
  if (selectedRange.value === 'custom' && startDate && endDate) {
    customDateRange.value = [startDate, endDate]
    return
  }
  customDateRange.value = buildPresetDateRange(selectedRange.value)
}

const isRangeTooLarge = (startDate: string, endDate: string) => {
  const start = new Date(startDate.replace(' ', 'T'))
  const end = new Date(endDate.replace(' ', 'T'))
  return end.getTime() - start.getTime() > MAX_CUSTOM_RANGE_DAYS * 24 * 60 * 60 * 1000
}

const disabledCustomDate = (date: Date) => {
  if (date.getTime() > Date.now()) {
    return true
  }
  if (!rangeAnchorDate.value) {
    return false
  }
  return Math.abs(date.getTime() - rangeAnchorDate.value.getTime()) > MAX_CUSTOM_RANGE_DAYS * 24 * 60 * 60 * 1000
}

const handleCustomCalendarChange = (value: Date[]) => {
  rangeAnchorDate.value = value?.[0] ?? null
}

const fetchProfile = async () => {
  if (!userId.value) return

  loading.value = true
  try {
    const res = await getUserPlaybackProfile(userId.value, {
      range: isCustomRange.value ? undefined : selectedRange.value,
      startDate: isCustomRange.value ? customDateRange.value?.[0] : undefined,
      endDate: isCustomRange.value ? customDateRange.value?.[1] : undefined
    })
    profile.value = res.data
  } catch (error) {
    profile.value = null
    if (error instanceof Error && error.message) {
      ElMessage.error(error.message)
    }
  } finally {
    loading.value = false
  }
}

const handleRangeChange = (range: PlaybackProfileRange) => {
  customDateRange.value = buildPresetDateRange(range)
  rangeAnchorDate.value = null
  router.replace({
    query: {
      ...route.query,
      range,
      startDate: undefined,
      endDate: undefined
    }
  })
}

const handleCustomRangeChange = (value: [string, string] | null) => {
  rangeAnchorDate.value = null
  if (!value || value.length !== 2) {
    const fallbackRange: PlaybackProfileRange = 'today'
    customDateRange.value = buildPresetDateRange(fallbackRange)
    router.replace({
      query: {
        ...route.query,
        range: fallbackRange,
        startDate: undefined,
        endDate: undefined
      }
    })
    return
  }

  if (isRangeTooLarge(value[0], value[1])) {
    ElMessage.warning('自定义日期范围最多选择 3 个月')
    return
  }

  customDateRange.value = value
  router.replace({
    query: {
      ...route.query,
      range: 'custom',
      startDate: value[0],
      endDate: value[1]
    }
  })
}

const handleBack = () => {
  if (window.history.length > 1) {
    router.back()
    return
  }
  router.push({ name: 'console-user-profiles' })
}

const handleViewHistory = () => {
  const username = profile.value?.username?.trim()
  const toDateOnly = (value: string) => value.slice(0, 10)
  const buildRangeQuery = () => {
    if (isCustomRange.value && customDateRange.value) {
      return {
        startDate: toDateOnly(customDateRange.value[0]),
        endDate: toDateOnly(customDateRange.value[1])
      }
    }
    const presetRange = buildPresetDateRange(selectedRange.value)
    if (!presetRange) return {}
    return {
      startDate: toDateOnly(presetRange[0]),
      endDate: toDateOnly(presetRange[1])
    }
  }
  router.push({
    name: 'console-playback-history',
    query: {
      username: username || undefined,
      userId: username ? undefined : userId.value,
      ...buildRangeQuery()
    }
  })
}

watch(
  () => [userId.value, route.query.range, route.query.startDate, route.query.endDate],
  () => {
    syncCustomRangeFromRoute()
    fetchProfile()
  },
  { immediate: true }
)
</script>

<template>
  <PlaybackProfileContent
    v-model:customDateRange="customDateRange"
    :title="profileTitle"
    description="从播放历史里提炼活跃时段、设备偏好和画像标签"
    :profile="profile"
    :loading="loading"
    :range-options="rangeOptions"
    :selected-range="selectedRange"
    :selected-range-label="selectedRangeLabel"
    :disabled-custom-date="disabledCustomDate"
    @range-change="handleRangeChange"
    @custom-calendar-change="handleCustomCalendarChange"
    @custom-range-change="handleCustomRangeChange"
  >
    <template #header-prefix>
      <div class="flex items-center gap-3">
        <button
          @click="handleBack"
          class="inline-flex shrink-0 whitespace-nowrap px-3 py-2 text-sm text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors cursor-pointer"
        >
          返回列表
        </button>
        <span class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">
          用户 ID: {{ userId || '-' }}
        </span>
      </div>
    </template>

    <template #toolbar-suffix>
      <button
        @click="handleViewHistory"
        class="inline-flex shrink-0 whitespace-nowrap px-4 py-2 text-sm rounded-xl border border-gray-200 bg-white text-gray-700 hover:bg-gray-50 transition-colors cursor-pointer"
      >
        播放历史
      </button>
    </template>
  </PlaybackProfileContent>
</template>
