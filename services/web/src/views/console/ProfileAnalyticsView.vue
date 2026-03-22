<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getProfileAnalytics } from '@/api/console'
import { formatPlaybackDate } from '@/utils/date'
import type {
  PlaybackProfileClientBucket,
  PlaybackProfileDeviceBucket,
  PlaybackProfileHourlyBucket,
  PlaybackProfileRange,
  UserPlaybackProfile
} from '@/types/api'

const router = useRouter()
const route = useRoute()

const loading = ref(false)
const profile = ref<UserPlaybackProfile | null>(null)

const rangeOptions: Array<{ label: string; value: PlaybackProfileRange }> = [
  { label: '近 7 天', value: '7d' },
  { label: '近 30 天', value: '30d' },
  { label: '近 90 天', value: '90d' },
  { label: '全部', value: 'all' }
]

const rangeLabelMap: Record<PlaybackProfileRange, string> = {
  '7d': '近 7 天',
  '30d': '近 30 天',
  '90d': '近 90 天',
  all: '全部历史'
}

const normalizeRange = (value: unknown): PlaybackProfileRange => {
  const raw = String(value ?? '').trim()
  if (raw === '7d' || raw === '30d' || raw === '90d' || raw === 'all') {
    return raw
  }
  return '30d'
}

const selectedRange = computed<PlaybackProfileRange>(() => normalizeRange(route.query.range))
const selectedRangeLabel = computed(() => rangeLabelMap[selectedRange.value])
const profileTitle = computed(() => profile.value?.username || '我的画像')
const badgeItemsRaw = computed(() => profile.value?.badges || [])
const badgeItems = computed(() => badgeItemsRaw.value.slice(0, 4))
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
    return { height: '0%' }
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

const fetchProfile = async () => {
  loading.value = true
  try {
    const res = await getProfileAnalytics({ range: selectedRange.value })
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
  router.replace({
    query: {
      ...route.query,
      range
    }
  })
}

watch(
  () => selectedRange.value,
  () => {
    fetchProfile()
  },
  { immediate: true }
)
</script>

<template>
  <div class="space-y-6" v-loading="loading">
    <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900">{{ profileTitle }}</h1>
          <p class="mt-1 text-sm text-gray-500">从播放历史里提炼你的活跃时段、设备偏好和画像标签</p>
        </div>

        <div class="flex flex-wrap items-center justify-start gap-2 xl:justify-end">
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
          <button
            @click="router.push('/console/account')"
            class="px-4 py-2 text-sm rounded-xl border border-gray-200 bg-white text-gray-700 hover:bg-gray-50 transition-colors cursor-pointer"
          >
            返回账号中心
          </button>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">累计播放时长</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ profile?.totalPlayDurationFormatted || '0m' }}</p>
        <p class="mt-2 text-xs text-gray-500">时间窗口 {{ selectedRangeLabel }}</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">播放次数</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ profile?.totalPlayCount || 0 }}</p>
        <p class="mt-2 text-xs text-gray-500">平均单次 {{ profile?.averagePlayDurationFormatted || '0m' }}</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">活跃天数</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ profile?.activeDays || 0 }}</p>
        <p class="mt-2 text-xs text-gray-500">有播放的天数越多，节奏越稳定</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm text-gray-500">最近播放</p>
        <p class="mt-3 text-lg font-bold text-gray-900">{{ profile?.lastPlayedAt ? formatPlaybackDate(profile.lastPlayedAt) : '-' }}</p>
        <p class="mt-2 text-xs text-gray-500">只展示自己的最近一次播放</p>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[1.4fr_1fr]">
      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">活跃时段</h2>
            <p class="mt-1 text-sm text-gray-500">按一天中的 24 个小时分布聚合播放次数，看看你通常什么时候看片</p>
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
            <p class="mt-1 text-sm text-gray-500">系统只展示最有代表性的少量标签，避免信息噪音</p>
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
            <h2 class="text-lg font-semibold text-gray-900">设备偏好</h2>
            <p class="mt-1 text-sm text-gray-500">按播放时长倒序，看看你主要在哪些设备上观看</p>
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
          当前时间窗口内没有设备偏好数据
        </div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">客户端偏好</h2>
            <p class="mt-1 text-sm text-gray-500">按播放时长倒序，看看你更常用哪种客户端</p>
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
          当前时间窗口内没有客户端偏好数据
        </div>
      </div>
    </div>

    <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-gray-900">最近播放记录</h2>
          <p class="mt-1 text-sm text-gray-500">只保留最近 10 条记录，方便你快速回看最近看了什么</p>
        </div>
        <span
          class="rounded-full px-3 py-1 text-xs font-medium"
          :class="profile?.recentRecords.length ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'"
        >
          {{ profile?.recentRecords.length ? '有播放数据' : '无播放数据' }}
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
