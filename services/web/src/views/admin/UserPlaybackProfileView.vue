<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Calendar } from '@element-plus/icons-vue'
import { getUserPlaybackProfile } from '@/api/admin'
import { formatPlaybackDate } from '@/utils/date'
import type {
  PlaybackProfileClientBucket,
  PlaybackProfileDeviceBucket,
  PlaybackProfileHourlyBucket,
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
  { label: '近 7 天', value: '7d' },
  { label: '近 30 天', value: '30d' },
  { label: '近 90 天', value: '90d' }
]
const rangeLabelMap: Record<PlaybackProfileRange, string> = {
  '7d': '近 7 天',
  '30d': '近 30 天',
  '90d': '近 90 天',
  all: '全部历史',
  custom: '自定义范围'
}

const normalizeRange = (value: unknown): PlaybackProfileRange => {
  const raw = String(value ?? '').trim()
  if (raw === '7d' || raw === '30d' || raw === '90d' || raw === 'all' || raw === 'custom') {
    return raw
  }
  return '30d'
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
const hasData = computed(() => (profile.value?.totalPlayCount ?? 0) > 0)
const profileTitle = computed(() => profile.value?.username || '用户画像')
const badgeItemsRaw = computed(() => profile.value?.badges || [])
const badgeDisplayLimit = 4
const badgeItems = computed(() => badgeItemsRaw.value.slice(0, badgeDisplayLimit))
const hourlyDistribution = computed(() => profile.value?.hourlyDistribution || [])

const maxHourlyCount = computed(() => {
  const values = hourlyDistribution.value.map(item => item.count)
  return Math.max(1, ...values)
})

const maxDeviceDuration = computed(() => {
  const values = profile.value?.deviceDistribution.map(item => item.duration) ?? []
  return Math.max(1, ...values)
})

const maxClientDuration = computed(() => {
  const values = profile.value?.clientDistribution.map(item => item.duration) ?? []
  return Math.max(1, ...values)
})

const activeHourCount = computed(() => hourlyDistribution.value.filter(item => item.count > 0).length)
const totalHourlyPlays = computed(() => hourlyDistribution.value.reduce((sum, item) => sum + item.count, 0))
const peakHourBucket = computed(() => {
  const sorted = hourlyDistribution.value
    .filter(item => item.count > 0)
    .slice()
    .sort((a, b) => {
      if (b.count !== a.count) return b.count - a.count
      return a.hour - b.hour
    })
  return sorted[0] ?? null
})
const nightActivityCount = computed(() =>
  hourlyDistribution.value
    .filter(item => item.hour >= 0 && item.hour < 6)
    .reduce((sum, item) => sum + item.count, 0)
)

const formatHourLabel = (hour: number): string => `${String(hour).padStart(2, '0')}:00`
const formatHourRangeLabel = (hour: number): string => `${formatHourLabel(hour)} - ${formatHourLabel((hour + 1) % 24)}`
const isPeakHour = (hour: number): boolean => peakHourBucket.value?.hour === hour && (peakHourBucket.value?.count || 0) > 0
const hourBarStyle = (item: PlaybackProfileHourlyBucket) => {
  if (item.count <= 0) {
    return {
      height: '0%'
    }
  }

  return {
    height: `${Math.max(12, Math.round((item.count / maxHourlyCount.value) * 100))}%`
  }
}

const distributionBarStyle = (
  item: PlaybackProfileDeviceBucket | PlaybackProfileClientBucket,
  max: number
) => ({
  width: `${Math.max(6, Math.round((item.duration / max) * 100))}%`
})

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
    const fallbackRange: PlaybackProfileRange = '30d'
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
  <div class="space-y-6" v-loading="loading">
    <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div class="min-w-0">
          <div class="flex items-center gap-3">
            <button
              @click="handleBack"
              class="px-3 py-2 text-sm text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors cursor-pointer"
            >
              返回用户管理
            </button>
            <span class="inline-flex items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">
              用户 ID: {{ userId || '-' }}
            </span>
          </div>

          <div class="mt-4">
            <h1 class="text-2xl font-bold text-gray-900">{{ profileTitle }}</h1>
            <p class="mt-1 text-sm text-gray-500">基于播放历史生成活跃概览、时段分布和设备偏好</p>
          </div>
        </div>

        <div class="flex min-w-0 flex-wrap items-center gap-2 xl:flex-nowrap xl:justify-end xl:max-w-[68rem] xl:mr-4">
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <button
              v-for="option in rangeOptions"
              :key="option.value"
              @click="handleRangeChange(option.value)"
              class="px-4 py-2 text-sm rounded-xl border transition-colors cursor-pointer"
              :class="selectedRange === option.value
                ? 'border-ember bg-ember text-white'
                : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'"
              >
                {{ option.label }}
              </button>
          </div>
          <div class="relative min-w-0 flex-1 w-full xl:basis-[46rem] xl:max-w-[46rem] group">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none z-10">
              <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Calendar /></el-icon>
            </div>
            <el-date-picker
              v-model="customDateRange"
              type="datetimerange"
              start-placeholder="开始日期时间"
              end-placeholder="结束日期时间"
              value-format="YYYY-MM-DD HH:mm:ss"
              class="w-full filter-date-range"
              unlink-panels
              clearable
              :disabled-date="disabledCustomDate"
              @calendar-change="handleCustomCalendarChange"
              @change="handleCustomRangeChange"
            />
          </div>
          <button
            @click="handleViewHistory"
            class="shrink-0 px-4 py-2 text-sm rounded-xl border border-gray-200 bg-white text-gray-700 hover:bg-gray-50 transition-colors cursor-pointer"
          >
            查看播放历史
          </button>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">累计播放时长</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ profile?.totalPlayDurationFormatted || '0m' }}</p>
        <p class="mt-2 text-xs text-gray-500">原始秒数 {{ profile?.totalPlayDuration || 0 }}</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">播放次数</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ profile?.totalPlayCount || 0 }}</p>
        <p class="mt-2 text-xs text-gray-500">平均单次 {{ profile?.averagePlayDurationFormatted || '0m' }}</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">活跃天数</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ profile?.activeDays || 0 }}</p>
        <p class="mt-2 text-xs text-gray-500">时间窗口 {{ selectedRange }}</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">最近播放</p>
        <p class="mt-3 text-lg font-bold text-gray-900">{{ profile?.lastPlayedAt ? formatPlaybackDate(profile.lastPlayedAt) : '-' }}</p>
        <p class="mt-2 text-xs text-gray-500">无播放记录时保持为空</p>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[1.4fr_1fr]">
      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">活跃时段</h2>
            <p class="mt-1 text-sm text-gray-500">按一天中的 24 个小时分布聚合播放次数，统计范围取当前时间窗口</p>
          </div>
          <div class="flex flex-wrap justify-end gap-2">
            <span class="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">24 小时分布</span>
            <span class="rounded-full bg-ember/10 px-3 py-1 text-xs font-medium text-ember">{{ selectedRangeLabel }}</span>
          </div>
        </div>

        <div v-if="hourlyDistribution.length > 0" class="mt-5 flex flex-wrap gap-2">
          <div class="rounded-full bg-gray-100 px-3 py-1.5 text-xs text-gray-700">
            峰值时段：
            <span class="font-semibold text-gray-900">
              {{ peakHourBucket ? `${formatHourRangeLabel(peakHourBucket.hour)} · ${peakHourBucket.count} 次` : '暂无' }}
            </span>
          </div>
          <div class="rounded-full bg-gray-100 px-3 py-1.5 text-xs text-gray-700">
            活跃小时：
            <span class="font-semibold text-gray-900">{{ activeHourCount }} / 24</span>
          </div>
          <div class="rounded-full bg-gray-100 px-3 py-1.5 text-xs text-gray-700">
            凌晨活动：
            <span class="font-semibold text-gray-900">{{ nightActivityCount }} 次</span>
          </div>
          <div class="rounded-full bg-gray-100 px-3 py-1.5 text-xs text-gray-700">
            总播放：
            <span class="font-semibold text-gray-900">{{ totalHourlyPlays }} 次</span>
          </div>
        </div>

        <div v-if="hourlyDistribution.length > 0" class="mt-6 h-64">
          <div class="flex h-full items-end gap-2">
            <div
              v-for="item in hourlyDistribution"
              :key="item.hour"
              class="flex h-full min-w-0 flex-1 flex-col justify-end gap-2"
            >
              <el-tooltip
                :content="`${formatHourRangeLabel(item.hour)}：${item.count} 次播放`"
                placement="top"
                :show-after="80"
              >
                <div class="flex flex-1 items-end rounded-2xl bg-gray-100/80 px-1 pt-4">
                  <div
                    class="w-full rounded-t-xl transition-all duration-300"
                    :class="[
                      item.count > 0
                        ? (isPeakHour(item.hour) ? 'bg-gradient-to-t from-slate-900 to-slate-500 shadow-sm' : 'bg-gradient-to-t from-ember to-red-300')
                        : 'bg-transparent',
                      item.count > 0 ? 'min-h-[12px]' : 'min-h-0'
                    ]"
                    :style="hourBarStyle(item)"
                  ></div>
                </div>
              </el-tooltip>
              <div class="text-center">
                <p class="text-[11px] font-semibold" :class="isPeakHour(item.hour) ? 'text-slate-900' : 'text-gray-600'">
                  {{ String(item.hour).padStart(2, '0') }}
                </p>
                <p class="text-[11px]" :class="item.count > 0 ? 'text-gray-500' : 'text-gray-300'">{{ item.count }}</p>
              </div>
            </div>
          </div>
        </div>

        <div v-else class="mt-8 rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-8 text-center text-sm text-gray-500">
          当前时间窗口内没有活跃时段数据
        </div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">画像标签</h2>
            <p class="mt-1 text-sm text-gray-500">默认只展示最有代表性的少量标签，避免把同类特征全部堆出来</p>
          </div>
          <span class="inline-flex shrink-0 items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium leading-none tabular-nums text-gray-600 whitespace-nowrap">
            {{ badgeItems.length }} / {{ badgeItemsRaw.length }}
          </span>
        </div>

        <div v-if="badgeItems.length > 0" class="mt-6 grid grid-cols-1 gap-3 2xl:grid-cols-2">
          <div
            v-for="badge in badgeItems"
            :key="badge.id"
            class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-4"
          >
            <p class="text-sm font-semibold text-gray-900">{{ badge.name }}</p>
            <p class="mt-2 text-sm leading-6 text-gray-600">{{ badge.description }}</p>
            <div class="mt-3 flex items-center gap-2">
              <span class="text-[11px] text-gray-400">标签 ID</span>
              <span class="rounded-full bg-white px-2.5 py-1 text-[11px] font-medium text-gray-500">{{ badge.id }}</span>
            </div>
          </div>
        </div>

        <div v-else class="mt-8 rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-8 text-center text-sm text-gray-500">
          当前时间窗口内还没有命中画像标签
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">设备分布</h2>
            <p class="mt-1 text-sm text-gray-500">按播放时长倒序，优先看真正常用设备</p>
          </div>
          <span class="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">
            {{ profile?.deviceDistribution.length || 0 }} 台
          </span>
        </div>

        <div v-if="(profile?.deviceDistribution.length || 0) > 0" class="mt-6 space-y-4">
          <div v-for="item in profile?.deviceDistribution || []" :key="item.deviceName" class="space-y-2">
            <div class="flex items-center justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-gray-900">{{ item.deviceName }}</p>
                <p class="text-xs text-gray-500">播放 {{ item.count }} 次</p>
              </div>
              <span class="text-sm font-medium text-gray-700">{{ item.durationFormatted }}</span>
            </div>
            <div class="h-2 rounded-full bg-gray-100">
              <div class="h-2 rounded-full bg-ember" :style="distributionBarStyle(item, maxDeviceDuration)"></div>
            </div>
          </div>
        </div>

        <div v-else class="mt-8 rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-8 text-center text-sm text-gray-500">
          当前时间窗口内没有设备分布数据
        </div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">客户端分布</h2>
            <p class="mt-1 text-sm text-gray-500">按播放时长倒序，排除只打开一次的噪音</p>
          </div>
          <span class="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">
            {{ profile?.clientDistribution.length || 0 }} 个
          </span>
        </div>

        <div v-if="(profile?.clientDistribution.length || 0) > 0" class="mt-6 space-y-4">
          <div v-for="item in profile?.clientDistribution || []" :key="item.clientName" class="space-y-2">
            <div class="flex items-center justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-gray-900">{{ item.clientName }}</p>
                <p class="text-xs text-gray-500">播放 {{ item.count }} 次</p>
              </div>
              <span class="text-sm font-medium text-gray-700">{{ item.durationFormatted }}</span>
            </div>
            <div class="h-2 rounded-full bg-gray-100">
              <div class="h-2 rounded-full bg-slate-700" :style="distributionBarStyle(item, maxClientDuration)"></div>
            </div>
          </div>
        </div>

        <div v-else class="mt-8 rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-8 text-center text-sm text-gray-500">
          当前时间窗口内没有客户端分布数据
        </div>
      </div>
    </div>

    <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-gray-900">最近播放记录</h2>
          <p class="mt-1 text-sm text-gray-500">保留最近 10 条记录，详情仍然回到播放历史查看</p>
        </div>
        <span
          class="rounded-full px-3 py-1 text-xs font-medium"
          :class="hasData ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'"
        >
          {{ hasData ? '有播放数据' : '无播放数据' }}
        </span>
      </div>

      <div v-if="(profile?.recentRecords.length || 0) > 0" class="mt-6 space-y-3">
        <div
          v-for="record in profile?.recentRecords || []"
          :key="`${record.itemName}-${record.playedAt}-${record.deviceName}`"
          class="rounded-2xl border border-gray-100 bg-gray-50/70 px-4 py-4"
        >
          <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div class="min-w-0">
              <p class="truncate text-sm font-semibold text-gray-900">{{ record.itemName || '未知片名' }}</p>
              <p class="mt-1 text-xs text-gray-500">
                {{ record.itemType || '未知类型' }} · {{ record.deviceName || '未知设备' }} · {{ record.clientName || '未知客户端' }}
              </p>
            </div>
            <div class="flex flex-wrap items-center gap-3 text-xs text-gray-500">
              <span>{{ record.playedAt ? formatPlaybackDate(record.playedAt) : '-' }}</span>
              <span class="rounded-full bg-white px-2.5 py-1 text-gray-700">{{ record.playDurationFormatted }}</span>
            </div>
          </div>
        </div>
      </div>

      <div v-else class="mt-8 rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-8 text-center text-sm text-gray-500">
        当前时间窗口内没有播放记录
      </div>
    </div>
  </div>
</template>

<style scoped>
:deep(.filter-date-range .el-date-editor),
:deep(.filter-date-range .el-range-editor.el-input__wrapper),
:deep(.filter-date-range.el-range-editor.el-input__wrapper) {
  height: 42px !important;
  min-height: 42px !important;
  border-radius: 0.75rem !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  background-color: #f9fafb !important;
}

:deep(.filter-date-range .el-input__wrapper),
:deep(.filter-date-range.el-input__wrapper) {
  overflow: hidden;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
  padding-left: 2.5rem !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  background-color: #f9fafb !important;
}

:deep(.filter-date-range:hover),
:deep(.filter-date-range:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.filter-date-range.is-active),
:deep(.filter-date-range.is-active .el-input__wrapper) {
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
  background-color: #ffffff !important;
}

:deep(.filter-date-range .el-range-input) {
  font-size: 0.875rem;
  background-color: transparent;
  min-width: 11rem;
  width: 11rem;
}

:deep(.filter-date-range .el-time-panel-link) {
  display: none !important;
}

:deep(.filter-date-range .el-range-separator) {
  color: #6b7280;
  min-width: 1.5rem;
  justify-content: center;
}

:deep(.filter-date-range .el-range__icon) {
  opacity: 0;
  width: 0;
  margin: 0;
}
</style>
