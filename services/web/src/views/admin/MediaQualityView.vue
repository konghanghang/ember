<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Film, RefreshRight, Search, Cpu } from '@element-plus/icons-vue'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { getMediaQualityLibraries, getMediaQualityGroupDetails, getMediaQualityPoster, getMediaQualityReport, scanMediaQualityLibrary } from '@/api/admin'
import { formatDate } from '@/utils/date'
import type { MediaQualityLibrary, MediaQualityLowDetailItem, MediaQualityLowItem, MediaQualityReport } from '@/types/api'

const loadingLibraries = ref(false)
const loadingReport = ref(false)
const scanning = ref(false)
const libraries = ref<MediaQualityLibrary[]>([])
const selectedLibraryId = ref('')
const report = ref<MediaQualityReport | null>(null)
const placeholderPoster = 'https://via.placeholder.com/120x180?text=No+Poster'
const posterURLMap = ref<Record<string, string>>({})
const loadingPosterIDs = new Set<string>()
const objectURLs = new Set<string>()
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailItems = ref<MediaQualityLowDetailItem[]>([])
const detailTotal = ref(0)
const detailQuery = ref({
  page: 1,
  pageSize: 20
})
const detailGroup = ref<{ id: string; name: string } | null>(null)
const summaryPosterIDs = ref<string[]>([])
const detailPosterIDs = ref<string[]>([])
const query = ref({
  page: 1,
  pageSize: 20
})

const totalScannedItems = computed(() => {
  if (!report.value) return 0
  return report.value.resolutionDistribution.reduce((sum, item) => sum + item.count, 0)
})

const libraryOptions = computed(() => {
  return [{ id: 'all', name: '全部媒体库', type: 'all', itemCount: 0 }, ...libraries.value]
})

const handleLibraryChange = () => {
  query.value.page = 1
  loadReport(false)
}

const fetchLibraries = async () => {
  loadingLibraries.value = true
  try {
    const res = await getMediaQualityLibraries()
    libraries.value = res.data || []
    const hasSelected = libraries.value.some(item => item.id === selectedLibraryId.value)
    if (!hasSelected) {
      selectedLibraryId.value = libraries.value[0]?.id || ''
    }
  } finally {
    loadingLibraries.value = false
  }
}

const getPosterUrl = (posterItemId: string) => {
  const id = posterItemId?.trim()
  if (!id) return placeholderPoster
  return posterURLMap.value[id] || placeholderPoster
}

const extractPosterIDs = (items: Array<{ posterItemId: string }>) => {
  const ids = new Set<string>()
  for (const item of items) {
    const id = item.posterItemId?.trim()
    if (id) ids.add(id)
  }
  return Array.from(ids)
}

const cleanupPosterURLs = () => {
  const activeIDs = new Set<string>([
    ...summaryPosterIDs.value,
    ...(detailVisible.value ? detailPosterIDs.value : [])
  ])

  for (const [id, url] of Object.entries(posterURLMap.value)) {
    if (activeIDs.has(id) || url === placeholderPoster) continue
    URL.revokeObjectURL(url)
    objectURLs.delete(url)
    delete posterURLMap.value[id]
  }
}

const loadReport = async (force = false) => {
  if (!selectedLibraryId.value) {
    ElMessage.warning('请先选择媒体库')
    return
  }
  loadingReport.value = true
  try {
    const res = await getMediaQualityReport(selectedLibraryId.value, {
      force,
      page: query.value.page,
      pageSize: query.value.pageSize
    })
    report.value = res.data
    summaryPosterIDs.value = extractPosterIDs(res.data?.lowQualityItems || [])
    cleanupPosterURLs()
    void preloadPosterURLs(res.data?.lowQualityItems || [])
    if (force) {
      ElMessage.success('已强制刷新并更新报告')
    }
  } finally {
    loadingReport.value = false
  }
}

const scanNow = async () => {
  if (!selectedLibraryId.value) {
    ElMessage.warning('请先选择媒体库')
    return
  }
  scanning.value = true
  try {
    await scanMediaQualityLibrary(selectedLibraryId.value)
    await loadReport(false)
    ElMessage.success('扫描完成，报告已更新')
  } finally {
    scanning.value = false
  }
}

const handlePageChange = (page: number) => {
  query.value.page = page
  loadReport(false)
}

const handlePageSizeChange = (size: number) => {
  query.value.pageSize = size
  query.value.page = 1
  loadReport(false)
}

const resolveLegacyGroupId = (row: MediaQualityLowItem) => {
  const rawID = row.id?.trim()
  if (!rawID) return ''
  if (rawID.includes(':')) return rawID
  return row.itemType === 'Series' ? `series:${rawID}` : `movie:${rawID}`
}

const resolveGroupId = (row: MediaQualityLowItem) => {
  const groupID = row.groupId?.trim()
  if (groupID) return groupID
  return resolveLegacyGroupId(row)
}

const openGroupDetails = async (row: MediaQualityLowItem) => {
  const groupID = resolveGroupId(row)
  if (!groupID) {
    ElMessage.warning('当前条目缺少可用分组信息')
    return
  }

  detailGroup.value = { id: groupID, name: row.name }
  detailQuery.value.page = 1
  detailPosterIDs.value = []
  detailVisible.value = true
  await fetchGroupDetails()
}

const fetchGroupDetails = async () => {
  if (!detailGroup.value?.id) return
  detailLoading.value = true
  try {
    const res = await getMediaQualityGroupDetails(selectedLibraryId.value, detailGroup.value.id, {
      page: detailQuery.value.page,
      pageSize: detailQuery.value.pageSize
    })
    detailItems.value = res.data || []
    detailTotal.value = res.total || 0
    detailPosterIDs.value = extractPosterIDs(detailItems.value)
    cleanupPosterURLs()
    void preloadPosterURLs((res.data || []).map(item => ({ posterItemId: item.posterItemId })))
  } finally {
    detailLoading.value = false
  }
}

const handleDetailPageChange = (page: number) => {
  detailQuery.value.page = page
  fetchGroupDetails()
}

const handleDetailPageSizeChange = (size: number) => {
  detailQuery.value.pageSize = size
  detailQuery.value.page = 1
  fetchGroupDetails()
}

const preloadPosterURLs = async (items: Array<{ posterItemId: string }>) => {
  for (const item of items) {
    const id = item.posterItemId?.trim()
    if (!id) continue
    if (posterURLMap.value[id] || loadingPosterIDs.has(id)) continue

    loadingPosterIDs.add(id)
    try {
      const blob = await getMediaQualityPoster(id)
      if (blob && blob.size > 0) {
        const objectURL = URL.createObjectURL(blob)
        objectURLs.add(objectURL)
        posterURLMap.value[id] = objectURL
      } else {
        posterURLMap.value[id] = placeholderPoster
      }
    } catch {
      posterURLMap.value[id] = placeholderPoster
    } finally {
      loadingPosterIDs.delete(id)
    }
  }
}

watch(detailVisible, visible => {
  if (!visible) {
    detailItems.value = []
    detailPosterIDs.value = []
    cleanupPosterURLs()
  }
})

onMounted(async () => {
  await fetchLibraries()
  if (selectedLibraryId.value) {
    await loadReport(false)
  }
})

onBeforeUnmount(() => {
  for (const url of objectURLs) {
    URL.revokeObjectURL(url)
  }
  objectURLs.clear()
  loadingPosterIDs.clear()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard title="媒体库质量盘点" description="按媒体库统计分辨率、编码、HDR 分布并筛出低画质资源。">
      <template #titleSuffix>
        <span
          v-if="report"
          class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500"
        >
          低画质 {{ report.lowQualityTotal }}
        </span>
      </template>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1.2fr)]"
        content-class="contents"
        actions-class="hidden"
      >
        <EmberSelectField
          v-model="selectedLibraryId"
          label="媒体库"
          placeholder="选择媒体库"
          :icon="Film"
          filterable
          :loading="loadingLibraries"
          @change="handleLibraryChange"
        >
          <el-option
            v-for="library in libraryOptions"
            :key="library.id"
            :label="`${library.name} (${library.type})`"
            :value="library.id"
          />
        </EmberSelectField>

        <div class="flex items-end">
          <button
            type="button"
            class="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100"
            :disabled="loadingReport"
            @click="loadReport(false)"
          >
            <el-icon><Search /></el-icon>
            读取报告
          </button>
        </div>

        <div class="flex items-end gap-2">
          <button
            type="button"
            class="inline-flex flex-1 items-center justify-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100"
            :disabled="loadingReport"
            @click="loadReport(true)"
          >
            <el-icon><RefreshRight /></el-icon>
            强制刷新
          </button>
          <button
            type="button"
            class="btn-ember inline-flex flex-1 items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold"
            :disabled="scanning"
            @click="scanNow"
          >
            <el-icon><Cpu /></el-icon>
            手动重扫
          </button>
        </div>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <div v-if="report" class="grid grid-cols-1 gap-4 md:grid-cols-3">
      <EmberMetricCard title="已统计条目" :value="totalScannedItems" />
      <EmberMetricCard
        title="低画质条目"
        :value="report.lowQualityTotal"
        value-class="mt-3 text-3xl font-bold text-orange-600"
      />
      <EmberMetricCard
        title="扫描时间"
        :value="report.scanAt ? formatDate(report.scanAt) : '-'"
        value-class="mt-2 text-base font-semibold text-gray-900"
      />
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-3">
      <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
        <div class="border-b border-gray-100 px-5 py-4 text-sm font-semibold text-gray-900">分辨率分布</div>
        <el-table :data="report?.resolutionDistribution || []" v-loading="loadingReport" size="small">
          <el-table-column prop="resolution" label="分辨率" />
          <el-table-column prop="count" label="数量" width="100" />
        </el-table>
      </section>

      <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
        <div class="border-b border-gray-100 px-5 py-4 text-sm font-semibold text-gray-900">编码分布</div>
        <el-table :data="report?.codecDistribution || []" v-loading="loadingReport" size="small">
          <el-table-column prop="codec" label="编码" />
          <el-table-column prop="count" label="数量" width="100" />
        </el-table>
      </section>

      <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
        <div class="border-b border-gray-100 px-5 py-4 text-sm font-semibold text-gray-900">HDR 分布</div>
        <el-table :data="report?.hdrDistribution || []" v-loading="loadingReport" size="small">
          <el-table-column prop="type" label="类型" />
          <el-table-column prop="count" label="数量" width="100" />
        </el-table>
      </section>
    </div>

    <EmberTableCard :data="report?.lowQualityItems || []" :loading="loadingReport">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-gray-900">低画质清单（汇总）</h2>
            <p class="mt-1 text-xs text-gray-500">按当前媒体库筛出命中的低画质分组，并支持查看组内明细。</p>
          </div>
          <span
            v-if="report"
            class="rounded-full bg-orange-50 px-2.5 py-1 text-[11px] font-semibold text-orange-700"
          >
            共 {{ report.lowQualityTotal }} 条
          </span>
        </div>
      </template>

      <template #default>
        <el-table-column label="封面" width="92">
          <template #default="{ row }">
            <img
              :src="getPosterUrl(row.posterItemId)"
              :alt="`${row.name} 封面`"
              class="h-20 w-14 rounded-md border border-gray-200 bg-gray-50 object-cover"
              @error="(e: Event) => (((e.target as HTMLImageElement).src = placeholderPoster))"
            />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="220" />
        <el-table-column prop="itemType" label="类型" width="90" />
        <el-table-column prop="itemCount" label="条目数" width="90" />
        <el-table-column prop="resolution" label="分辨率" width="110" />
        <el-table-column prop="codec" label="编码" width="120" />
        <el-table-column prop="bitrate" label="码率(kbps)" width="120" />
        <el-table-column prop="id" label="条目 ID" min-width="220">
          <template #default="{ row }">
            <code class="text-xs text-gray-600">{{ row.id }}</code>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <button
              type="button"
              class="cursor-pointer text-sm font-semibold text-ember transition-colors hover:text-red-700 hover:underline"
              :aria-label="`查看 ${row.name} 低画质详情`"
              @click="openGroupDetails(row)"
            >
              查看详情
            </button>
          </template>
        </el-table-column>
      </template>

      <template #pagination>
        <el-pagination
          :current-page="query.page"
          :page-size="query.pageSize"
          :total="report?.lowQualityTotal || 0"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
          background
        />
      </template>
    </EmberTableCard>

    <el-drawer v-model="detailVisible" size="60%" destroy-on-close append-to-body>
      <template #header>
        <div class="text-lg font-bold text-gray-900">
          低画质明细：{{ detailGroup?.name || '-' }}
        </div>
      </template>

      <el-table :data="detailItems" v-loading="detailLoading" style="width: 100%">
        <el-table-column label="封面" width="92">
          <template #default="{ row }">
            <img
              :src="getPosterUrl(row.posterItemId)"
              :alt="`${row.name} 封面`"
              class="h-20 w-14 rounded-md border border-gray-200 bg-gray-50 object-cover"
              @error="(e: Event) => (((e.target as HTMLImageElement).src = placeholderPoster))"
            />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="条目名称" min-width="260" />
        <el-table-column prop="resolution" label="分辨率" width="110" />
        <el-table-column prop="codec" label="编码" width="120" />
        <el-table-column prop="bitrate" label="码率(kbps)" width="120" />
      </el-table>

      <div class="mt-4 flex justify-end">
        <el-pagination
          :current-page="detailQuery.page"
          :page-size="detailQuery.pageSize"
          :total="detailTotal"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="handleDetailPageChange"
          @size-change="handleDetailPageSizeChange"
          background
        />
      </div>
    </el-drawer>
  </div>
</template>
