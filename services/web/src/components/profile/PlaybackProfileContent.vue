<script setup lang="ts">
import { computed } from 'vue'
import EmberDateRangeField from '@/components/ember/filters/EmberDateRangeField.vue'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import { emberRangePickerPopperClass, rangePickerDefaultTime } from '@/constants/datePicker'
import { formatPlaybackDate } from '@/utils/date'
import type {
  PlaybackProfileClientBucket,
  PlaybackProfileDeviceBucket,
  PlaybackProfileHourlyBucket,
  PlaybackProfileRange,
  UserPlaybackProfile
} from '@/types/api'

type PlaybackProfileRangeOption = {
  label: string
  value: PlaybackProfileRange
}

const props = defineProps<{
  title: string
  /**
   * 历史复述型描述，已被 §2.2.1 文案克制规则废弃，不再渲染。
   * 仍保留为可选入参，避免调用方一次性迁移；待全部调用方清理后可删除。
   */
  description?: string
  profile: UserPlaybackProfile | null
  loading: boolean
  rangeOptions: PlaybackProfileRangeOption[]
  selectedRange: PlaybackProfileRange
  selectedRangeLabel: string
  customDateRange: [string, string] | null
  disabledCustomDate: (date: Date) => boolean
}>()

const emit = defineEmits<{
  'range-change': [value: PlaybackProfileRange]
  // Element Plus 的 calendar-change 载荷在公共类型中未标注，这里收 unknown 让调用方各自收窄。
  'custom-calendar-change': [value: unknown]
  // Element Plus change 载荷允许空数组/undefined（清空场景），透传给调用方判定。
  'custom-range-change': [value: [string, string] | [] | null | undefined]
  'update:customDateRange': [value: [string, string] | null]
}>()

const dateRangeModel = computed({
  get: () => props.customDateRange,
  set: (value: [string, string] | null) => emit('update:customDateRange', value)
})

// EmberSegmentTabs 用 string key 维护选中态；这里在边界收窄回业务类型，非法值兜底为 today。
const isKnownRange = (value: string): value is PlaybackProfileRange => {
  return ['today', '7d', '30d', '90d', 'all', 'custom'].includes(value)
}
const handleRangeSelect = (value: string) => {
  if (!isKnownRange(value)) return
  emit('range-change', value)
}

// rangeOptions 用于驱动 EmberSegmentTabs 的分段控件，统一接入 roving tabindex。
const rangeTabs = computed(() =>
  props.rangeOptions.map(option => ({ key: option.value, label: option.label }))
)

const badgeItemsRaw = computed(() => props.profile?.badges || [])
const badgeItems = computed(() => badgeItemsRaw.value.slice(0, 4))
const hourlyDistribution = computed(() => props.profile?.hourlyDistribution || [])
const hasData = computed(() => (props.profile?.totalPlayCount ?? 0) > 0)

const maxHourlyCount = computed(() => {
  const values = hourlyDistribution.value.map(item => item.count)
  return Math.max(1, ...values)
})

const maxDeviceDuration = computed(() => {
  const values = props.profile?.deviceDistribution.map(item => item.duration) ?? []
  return Math.max(1, ...values)
})

const maxClientDuration = computed(() => {
  const values = props.profile?.clientDistribution.map(item => item.duration) ?? []
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
</script>

<template>
  <div class="space-y-6" v-loading="loading">
    <EmberPageHeaderCard :title="title">
      <template #titleSuffix>
        <span class="rounded-full bg-ember/10 px-2.5 py-1 text-xs font-normal text-ember">
          {{ selectedRangeLabel }}
        </span>
      </template>

      <template #actions>
        <div class="flex w-full flex-col gap-3 md:w-auto md:flex-row md:flex-wrap md:items-center md:justify-end">
          <div class="w-full md:w-auto">
            <EmberSegmentTabs
              :model-value="selectedRange"
              :tabs="rangeTabs"
              :full-width="false"
              ariaLabel="画像时间窗口切换"
              @change="handleRangeSelect"
            />
          </div>

          <div class="w-full md:w-[26rem]">
            <EmberDateRangeField
              v-model="dateRangeModel"
              label="自定义范围"
              type="datetimerange"
              start-placeholder="开始日期时间"
              end-placeholder="结束日期时间"
              value-format="YYYY-MM-DD HH:mm:ss"
              :default-time="rangePickerDefaultTime"
              :popper-class="emberRangePickerPopperClass"
              unlink-panels
              clearable
              :disabled-date="disabledCustomDate"
              @calendar-change="emit('custom-calendar-change', $event)"
              @change="emit('custom-range-change', $event)"
            />
          </div>

          <slot name="toolbar-suffix" />
        </div>
      </template>

      <slot name="header-prefix" />

      <div class="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <EmberMetricCard
          title="累计播放时长"
          :value="profile?.totalPlayDurationFormatted || '0m'"
          :detail="`时间窗口 ${selectedRangeLabel}`"
        />

        <EmberMetricCard
          title="播放次数"
          :value="profile?.totalPlayCount || 0"
          :detail="`平均单次 ${profile?.averagePlayDurationFormatted || '0m'}`"
        />

        <EmberMetricCard
          title="活跃天数"
          :value="profile?.activeDays || 0"
        />

        <EmberMetricCard
          title="最近播放"
          :value="profile?.lastPlayedAt ? formatPlaybackDate(profile.lastPlayedAt) : '-'"
          value-class="mt-3 text-lg font-bold text-gray-900"
        />
      </div>
    </EmberPageHeaderCard>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[1.4fr_1fr]">
      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <h2 class="text-lg font-semibold text-gray-900">活跃时段</h2>
          <span class="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600">24 小时分布</span>
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
                        ? (isPeakHour(item.hour) ? 'bg-ember' : 'bg-ember/50')
                        : 'bg-transparent',
                      item.count > 0 ? 'min-h-[12px]' : 'min-h-0'
                    ]"
                    :style="hourBarStyle(item)"
                  ></div>
                </div>
              </el-tooltip>
              <div class="text-center">
                <p class="text-[11px] font-semibold" :class="isPeakHour(item.hour) ? 'text-gray-900' : 'text-gray-600'">
                  {{ String(item.hour).padStart(2, '0') }}
                </p>
                <p class="text-[11px]" :class="item.count > 0 ? 'text-gray-500' : 'text-gray-300'">{{ item.count }}</p>
              </div>
            </div>
          </div>
        </div>

        <EmberEmptyStateCard
          v-else
          class="mt-8"
          compact
          title="当前时间窗口内没有活跃时段数据"
        />
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <h2 class="text-lg font-semibold text-gray-900">画像标签</h2>
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
          </div>
        </div>

        <EmberEmptyStateCard
          v-else
          class="mt-8"
          compact
          title="当前时间窗口内还没有命中画像标签"
        />
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <h2 class="text-lg font-semibold text-gray-900">设备偏好</h2>
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

        <EmberEmptyStateCard
          v-else
          class="mt-8"
          compact
          title="当前时间窗口内没有设备偏好数据"
        />
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-start justify-between gap-4">
          <h2 class="text-lg font-semibold text-gray-900">客户端偏好</h2>
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
              <div class="h-2 rounded-full bg-ember" :style="distributionBarStyle(item, maxClientDuration)"></div>
            </div>
          </div>
        </div>

        <EmberEmptyStateCard
          v-else
          class="mt-8"
          compact
          title="当前时间窗口内没有客户端偏好数据"
        />
      </div>
    </div>

    <div class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <h2 class="text-lg font-semibold text-gray-900">最近播放记录</h2>
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

      <EmberEmptyStateCard
        v-else
        class="mt-8"
        compact
        title="当前时间窗口内没有播放记录"
      />
    </div>
  </div>
</template>