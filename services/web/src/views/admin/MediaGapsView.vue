<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Calendar,
  CircleCheckFilled,
  Clock,
  Collection,
  Download,
  Grid,
  InfoFilled,
  Loading,
  RefreshRight,
  Search,
  Upload
} from '@element-plus/icons-vue'
import {
  dispatchMediaGap,
  getMediaGapScanStatus,
  getMediaGaps,
  getGroupedMediaGaps,
  ignoreMediaGap,
  scanMediaGaps,
  searchMediaGap
} from '@/api/admin'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberDateRangeField from '@/components/ember/filters/EmberDateRangeField.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { formatDate } from '@/utils/date'
import type {
  MediaGapGroupedQuery,
  MediaGapGroupedResponse,
  MediaGapGroupedSeries,
  MediaGapGroupedSortMode,
  MediaGapItem,
  MediaGapListQuery,
  MediaGapScanStatus,
  MediaGapSearchCandidate,
  MediaGapSearchResult,
  MediaGapStatus
} from '@/types/api'

type MediaGapViewMode = 'grouped' | 'table'
type MediaGapSortMode = MediaGapGroupedSortMode

const loading = ref(false)
const scanning = ref(false)
const scanStatus = ref<MediaGapScanStatus>({
  status: 'idle',
  running: false,
  message: '暂无扫描任务'
})
const tableData = ref<MediaGapItem[]>([])
const groupedData = ref<MediaGapGroupedSeries[]>([])
const total = ref(0)
const itemTotal = ref(0)
const groupedSummary = ref<MediaGapGroupedResponse['summary']>({
  missingCount: 0,
  searchedCount: 0,
  requestedCount: 0,
  ingestedCount: 0,
  ignoredCount: 0
})
const airDateRange = ref<[string, string] | null>(null)
const dialogVisible = ref(false)
const dialogLoading = ref(false)
const dispatching = ref(false)
const currentGap = ref<MediaGapItem | null>(null)
const candidateResult = ref<MediaGapSearchResult>({
  candidates: []
})
const selectedCandidateId = ref('')
const selectedGapId = ref('')
const viewMode = ref<MediaGapViewMode>('grouped')
const sortMode = ref<MediaGapSortMode>('missing')
const expandedSeasonKeys = ref<string[]>([])
const expandedSeriesKeys = ref<string[]>([])
let scanStatusPollTimer: ReturnType<typeof setTimeout> | null = null

const queryParams = ref<MediaGapListQuery>({
  page: 1,
  pageSize: 9,
  keyword: '',
  status: ''
})

const groupedPageSizes = [9, 18, 36]
const tablePageSizes = [20, 50, 100]

const statusOptions: Array<{ label: string; value: MediaGapStatus }> = [
  { label: '待处理', value: 'MISSING' },
  { label: '已搜索', value: 'SEARCHED' },
  { label: '已下发', value: 'REQUESTED' },
  { label: '下发失败', value: 'DISPATCH_FAILED' },
  { label: '已入库', value: 'INGESTED' },
  { label: '已忽略', value: 'IGNORED' }
]

const statusMeta: Record<MediaGapStatus, { label: string; type: '' | 'success' | 'info' | 'warning' | 'danger' | 'primary' }> = {
  MISSING: { label: '待处理', type: 'danger' },
  SEARCHED: { label: '已搜索', type: 'warning' },
  REQUESTED: { label: '已下发', type: 'primary' },
  DISPATCH_FAILED: { label: '下发失败', type: 'danger' },
  INGESTED: { label: '已入库', type: 'success' },
  IGNORED: { label: '已忽略', type: 'info' }
}

const sortOptions: Array<{ label: string; value: MediaGapSortMode; hint: string }> = [
  { label: '缺口优先', value: 'missing', hint: '先看缺集最多的剧' },
  { label: '最近变化', value: 'updated', hint: '先看最近有状态变化的剧' },
  { label: '已下发优先', value: 'requested', hint: '先盯等待入库的剧' },
  { label: '剧名字母', value: 'name', hint: '按剧名排序浏览' }
]

const dialogTitle = '搜索候选并下发'

const selectedCandidate = computed(() => {
  return candidateResult.value.candidates.find((candidate) => candidate.id === selectedCandidateId.value) ?? null
})

const canDispatch = computed(() => {
  return Boolean(currentGap.value && selectedCandidate.value && !dispatching.value)
})

const seriesCount = computed(() => {
  return viewMode.value === 'grouped' ? total.value : tableData.value.length
})
const missingCount = computed(() => {
  return viewMode.value === 'grouped'
    ? groupedSummary.value.missingCount
    : tableData.value.filter((item) => item.status === 'MISSING').length
})
const searchedCount = computed(() => {
  return viewMode.value === 'grouped'
    ? groupedSummary.value.searchedCount
    : tableData.value.filter((item) => item.status === 'SEARCHED').length
})
const requestedCount = computed(() => {
  return viewMode.value === 'grouped'
    ? groupedSummary.value.requestedCount
    : tableData.value.filter((item) => item.status === 'REQUESTED').length
})
const ingestedCount = computed(() => {
  return viewMode.value === 'grouped'
    ? groupedSummary.value.ingestedCount
    : tableData.value.filter((item) => item.status === 'INGESTED').length
})
const ignoredCount = computed(() => {
  return viewMode.value === 'grouped'
    ? groupedSummary.value.ignoredCount
    : tableData.value.filter((item) => item.status === 'IGNORED').length
})
const dispatchFailedCount = computed(() => {
  return viewMode.value === 'grouped'
    ? (groupedSummary.value.dispatchFailedCount ?? 0)
    : tableData.value.filter((item) => item.status === 'DISPATCH_FAILED').length
})

const compactStats = computed(() => [
  {
    label: '工单',
    value: itemTotal.value,
    tone: 'neutral'
  },
  {
    label: viewMode.value === 'grouped' ? '剧集' : '当前页剧集',
    value: seriesCount.value,
    tone: 'neutral'
  },
  {
    label: '待处理',
    value: missingCount.value,
    tone: 'missing'
  },
  {
    label: '已下发',
    value: requestedCount.value,
    tone: 'requested'
  },
  {
    label: '下发失败',
    value: dispatchFailedCount.value,
    tone: 'missing'
  },
  {
    label: '已完成',
    value: ingestedCount.value,
    tone: 'settled'
  },
  {
    label: '已忽略',
    value: ignoredCount.value,
    tone: 'muted'
  }
])

const currentPageSizes = computed(() => {
  return viewMode.value === 'grouped' ? groupedPageSizes : tablePageSizes
})

const defaultPageSize = computed(() => {
  return viewMode.value === 'grouped' ? groupedPageSizes[0] : tablePageSizes[0]
})

const isTerminalStatus = (status: MediaGapStatus) => status === 'INGESTED' || status === 'IGNORED'
const isMessageBoxCancel = (error: unknown) => error === 'cancel' || error === 'close'

const clearScanStatusPoll = () => {
  if (scanStatusPollTimer) {
    clearTimeout(scanStatusPollTimer)
    scanStatusPollTimer = null
  }
}

const applyScanStatus = (status: MediaGapScanStatus) => {
  scanStatus.value = status
  scanning.value = status.running
}

const scheduleScanStatusPoll = () => {
  clearScanStatusPoll()
  if (!scanStatus.value.running) {
    return
  }
  scanStatusPollTimer = setTimeout(() => {
    void refreshScanStatus(true)
  }, 3000)
}

const refreshScanStatus = async (notifyCompletion = false) => {
  const previous = scanStatus.value

  try {
    const res = await getMediaGapScanStatus()
    const nextStatus = res.data ?? {
      status: 'idle',
      running: false,
      message: '暂无扫描任务'
    }

    applyScanStatus(nextStatus)

    if (
      notifyCompletion &&
      previous.running &&
      previous.scanId &&
      previous.scanId === nextStatus.scanId &&
      !nextStatus.running
    ) {
      if (nextStatus.status === 'succeeded') {
        ElMessage.success(nextStatus.message || '缺集扫描完成')
        await fetchData()
      } else if (nextStatus.status === 'failed') {
        ElMessage.error(nextStatus.error || nextStatus.message || '缺集扫描失败')
      }
    }
  } catch {
    if (!scanStatus.value.running) {
      scanning.value = false
    }
  } finally {
    scheduleScanStatusPoll()
  }
}

const formatDateOnly = (value?: string) => {
  const raw = String(value ?? '').trim()
  if (!raw) return '-'

  const match = raw.match(/^(\d{4})-(\d{2})-(\d{2})/)
  if (match) {
    return `${match[1]}/${match[2]}/${match[3]}`
  }

  return formatDate(raw)
}

const formatDateTime = (value?: string) => {
  const raw = String(value ?? '').trim()
  if (!raw) return '-'

  const plainDateTime = raw.match(/^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})/)
  if (plainDateTime) {
    return `${plainDateTime[1]}/${plainDateTime[2]}/${plainDateTime[3]} ${plainDateTime[4]}:${plainDateTime[5]}`
  }

  return formatDate(raw)
}

const formatEpisodeCode = (row: Pick<MediaGapItem, 'season' | 'episode'>) => {
  return `S${String(row.season).padStart(2, '0')}E${String(row.episode).padStart(2, '0')}`
}

const formatSeasonCode = (season: number) => {
  return `S${String(season).padStart(2, '0')}`
}

const seasonExpandKey = (seriesKey: string, season: number) => `${seriesKey}:${season}`

const isSeasonExpanded = (seriesKey: string, season: number) => {
  return expandedSeasonKeys.value.includes(seasonExpandKey(seriesKey, season))
}

const toggleSeasonExpanded = (seriesKey: string, season: number) => {
  const key = seasonExpandKey(seriesKey, season)
  if (expandedSeasonKeys.value.includes(key)) {
    expandedSeasonKeys.value = expandedSeasonKeys.value.filter((item) => item !== key)
    return
  }
  expandedSeasonKeys.value = [...expandedSeasonKeys.value, key]
}

const visibleSeasonGaps = (seriesKey: string, seasonGroup: { season: number; gaps: MediaGapItem[] }) => {
  const defaultVisible = 12
  if (isSeasonExpanded(seriesKey, seasonGroup.season)) {
    return seasonGroup.gaps
  }
  return seasonGroup.gaps.slice(0, defaultVisible)
}

const hiddenSeasonGapCount = (seriesKey: string, seasonGroup: { season: number; gaps: MediaGapItem[] }) => {
  return Math.max(0, seasonGroup.gaps.length - visibleSeasonGaps(seriesKey, seasonGroup).length)
}

const isSeriesExpanded = (seriesKey: string) => expandedSeriesKeys.value.includes(seriesKey)

const toggleSeriesExpanded = (seriesKey: string) => {
  if (expandedSeriesKeys.value.includes(seriesKey)) {
    expandedSeriesKeys.value = expandedSeriesKeys.value.filter((item) => item !== seriesKey)
    return
  }
  expandedSeriesKeys.value = [...expandedSeriesKeys.value, seriesKey]
}

const actionableSeasonGroups = (series: MediaGapGroupedSeries) => {
  return series.seasons
    .map((seasonGroup) => ({
      season: seasonGroup.season,
      gaps: seasonGroup.gaps.filter((gap) => !isTerminalStatus(gap.status))
    }))
    .filter((seasonGroup) => seasonGroup.gaps.length > 0)
}

const visibleSeasonGroups = (series: MediaGapGroupedSeries) => {
  const defaultVisibleSeasons = 1
  const actionableSeasons = actionableSeasonGroups(series)
  if (isSeriesExpanded(series.key)) {
    return actionableSeasons
  }
  return actionableSeasons.slice(0, defaultVisibleSeasons)
}

const hiddenSeasonGroupCount = (series: MediaGapGroupedSeries) => {
  return Math.max(0, actionableSeasonGroups(series).length - visibleSeasonGroups(series).length)
}

const statusOrder = (status: MediaGapStatus) => {
  switch (status) {
    case 'MISSING':
      return 0
    case 'SEARCHED':
      return 1
    case 'REQUESTED':
      return 2
    case 'DISPATCH_FAILED':
      return 3
    case 'INGESTED':
      return 4
    case 'IGNORED':
      return 5
    default:
      return 9
  }
}

const compactStatClass = (tone: string) => {
  switch (tone) {
    case 'missing':
      return 'compact-stat compact-stat-missing'
    case 'requested':
      return 'compact-stat compact-stat-requested'
    case 'settled':
      return 'compact-stat compact-stat-settled'
    case 'muted':
      return 'compact-stat compact-stat-muted'
    default:
      return 'compact-stat compact-stat-neutral'
  }
}

const episodeChipClass = (gap: MediaGapItem) => {
  const classes = ['episode-chip']
  if (selectedGapId.value === gap.id) classes.push('episode-chip-selected')

  switch (gap.status) {
    case 'MISSING':
      classes.push('episode-chip-missing')
      break
    case 'SEARCHED':
      classes.push('episode-chip-searched')
      break
    case 'REQUESTED':
      classes.push('episode-chip-requested')
      break
    case 'DISPATCH_FAILED':
      classes.push('episode-chip-dispatch-failed')
      break
    case 'INGESTED':
      classes.push('episode-chip-ingested')
      break
    case 'IGNORED':
      classes.push('episode-chip-ignored')
      break
  }

  return classes.join(' ')
}

const resolveActiveGap = (group: MediaGapGroupedSeries) => {
  const actionableGaps = group.gaps.filter((gap) => !isTerminalStatus(gap.status))
  const selected = actionableGaps.find((gap) => gap.id === selectedGapId.value)
  return selected ?? actionableGaps[0] ?? null
}

const selectGap = (gap: MediaGapItem) => {
  selectedGapId.value = gap.id
}

const patchGap = (gap?: MediaGapItem | null) => {
  if (!gap?.id) return

  const index = tableData.value.findIndex((item) => item.id === gap.id)
  if (index >= 0) {
    tableData.value[index] = {
      ...tableData.value[index],
      ...gap
    }
  }

  if (currentGap.value?.id === gap.id) {
    currentGap.value = {
      ...(currentGap.value || gap),
      ...gap
    }
  }
}

const parseJSONSafely = (value: unknown) => {
  if (typeof value !== 'string') return value

  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

const formatCandidateSize = (value: unknown) => {
  if (value === null || value === undefined) return undefined

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return undefined

    const numeric = Number(trimmed)
    if (!Number.isFinite(numeric)) {
      return trimmed
    }

    value = numeric
  }

  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return undefined
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }

  const precision = size >= 100 || unitIndex === 0 ? 0 : size >= 10 ? 1 : 2
  return `${size.toFixed(precision)} ${units[unitIndex]}`
}

const normalizeCandidate = (value: unknown, index: number): MediaGapSearchCandidate => {
  if (typeof value === 'string') {
    return {
      id: `candidate-${index}`,
      title: value
    }
  }

  if (!value || typeof value !== 'object') {
    return {
      id: `candidate-${index}`,
      title: `候选 ${index + 1}`
    }
  }

  const record = value as Record<string, unknown>
  const title = String(record.title ?? record.name ?? record.resourceTitle ?? record.subject ?? `候选 ${index + 1}`).trim()
  const subtitle = String(record.subtitle ?? record.subTitle ?? record.quality ?? '').trim()
  const publishDate = String(record.publishDate ?? record.pubDate ?? record.publishedAt ?? '').trim()
  const source = String(record.source ?? record.provider ?? record.channel ?? '').trim()
  const site = String(record.site ?? record.siteName ?? '').trim()
  const size = formatCandidateSize(record.size ?? record.sizeLabel)
  const language = String(record.language ?? '').trim()
  const releaseGroup = String(record.releaseGroup ?? record.team ?? '').trim()
  const episodeRange = String(record.episodeRange ?? record.episodes ?? '').trim()
  const matchReason = String(record.matchReason ?? record.matchedBy ?? '').trim()
  const description = String(record.description ?? record.summary ?? record.overview ?? '').trim()
  const idSource = record.id ?? record.candidateId ?? record.guid ?? record.hash ?? title
  const seedersValue = record.seeders
  const seeders = typeof seedersValue === 'number' ? seedersValue : Number.parseInt(String(seedersValue ?? ''), 10)
  const payload = record.payload && typeof record.payload === 'object' ? (record.payload as Record<string, unknown>) : undefined

  return {
    id: String(idSource),
    title,
    subtitle: subtitle || undefined,
    source: source || undefined,
    site: site || undefined,
    size,
    seeders: Number.isFinite(seeders) ? seeders : undefined,
    publishDate: publishDate || undefined,
    language: language || undefined,
    releaseGroup: releaseGroup || undefined,
    episodeRange: episodeRange || undefined,
    matchReason: matchReason || undefined,
    description: description || undefined,
    payload
  }
}

const normalizeSearchResult = (payload: unknown, gap?: MediaGapItem | null): MediaGapSearchResult => {
  const parsed = parseJSONSafely(payload)

  if (Array.isArray(parsed)) {
    return {
      mediaGap: gap ?? undefined,
      candidates: parsed.map((item, index) => normalizeCandidate(item, index))
    }
  }

  if (!parsed || typeof parsed !== 'object') {
    return {
      mediaGap: gap ?? undefined,
      candidates: []
    }
  }

  const record = parsed as Record<string, unknown>
  const nestedSnapshot = parseJSONSafely(record.searchSnapshot)
  const rawCandidates =
    record.candidates ??
    record.items ??
    record.results ??
    record.list ??
    (nestedSnapshot && typeof nestedSnapshot === 'object' ? (nestedSnapshot as Record<string, unknown>).candidates : undefined) ??
    []

  return {
    mediaGap: (record.mediaGap as MediaGapItem | undefined) ?? gap ?? undefined,
    candidates: Array.isArray(rawCandidates) ? rawCandidates.map((item, index) => normalizeCandidate(item, index)) : [],
    searchedAt: String(
      record.searchedAt ??
      record.lastSearchedAt ??
      (nestedSnapshot && typeof nestedSnapshot === 'object' ? (nestedSnapshot as Record<string, unknown>).searchedAt ?? '' : '')
    ).trim() || undefined,
    source: String(
      record.source ??
      record.provider ??
      (nestedSnapshot && typeof nestedSnapshot === 'object' ? (nestedSnapshot as Record<string, unknown>).source ?? '' : '')
    ).trim() || undefined
  }
}

const buildParams = (): MediaGapListQuery => {
  const params: MediaGapListQuery = {
    page: queryParams.value.page,
    pageSize: queryParams.value.pageSize
  }

  if (queryParams.value.keyword?.trim()) {
    params.keyword = queryParams.value.keyword.trim()
  }
  if (queryParams.value.status) {
    params.status = queryParams.value.status
  }
  if (airDateRange.value && airDateRange.value.length === 2) {
    params.airDateFrom = airDateRange.value[0]
    params.airDateTo = airDateRange.value[1]
  }

  return params
}

const buildGroupedParams = () => {
  const params: MediaGapGroupedQuery = {
    page: queryParams.value.page,
    pageSize: queryParams.value.pageSize,
    sort: sortMode.value
  }

  if (queryParams.value.keyword?.trim()) {
    params.keyword = queryParams.value.keyword.trim()
  }
  if (queryParams.value.status) {
    params.status = queryParams.value.status
  }
  if (airDateRange.value && airDateRange.value.length === 2) {
    params.airDateFrom = airDateRange.value[0]
    params.airDateTo = airDateRange.value[1]
  }

  return params
}

const fetchData = async () => {
  loading.value = true
  try {
    if (viewMode.value === 'grouped') {
      const res = await getGroupedMediaGaps(buildGroupedParams())
      groupedData.value = res.data ?? []
      tableData.value = groupedData.value.flatMap((series) => series.gaps)
      total.value = res.total ?? 0
      itemTotal.value = res.itemTotal ?? tableData.value.length
      groupedSummary.value = res.summary ?? {
        missingCount: 0,
        searchedCount: 0,
        requestedCount: 0,
        ingestedCount: 0,
        ignoredCount: 0
      }

      if (selectedGapId.value && !tableData.value.some((item) => item.id === selectedGapId.value)) {
        selectedGapId.value = ''
      }
      if (!selectedGapId.value && tableData.value.length > 0) {
        selectedGapId.value = tableData.value[0].id
      }
      return
    }

    const res = await getMediaGaps(buildParams())
    groupedData.value = []
    tableData.value = res.data ?? []
    total.value = res.total ?? 0
    itemTotal.value = res.total ?? 0
    groupedSummary.value = {
      missingCount: 0,
      searchedCount: 0,
      requestedCount: 0,
      ingestedCount: 0,
      ignoredCount: 0
    }

    if (selectedGapId.value && !tableData.value.some((item) => item.id === selectedGapId.value)) {
      selectedGapId.value = ''
    }
    if (!selectedGapId.value && tableData.value.length > 0) {
      selectedGapId.value = tableData.value[0].id
    }
  } catch {
    // handled by interceptor
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleReset = () => {
  queryParams.value = {
    page: 1,
    pageSize: defaultPageSize.value,
    keyword: '',
    status: ''
  }
  airDateRange.value = null
  fetchData()
}

const handlePageSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const handlePageChange = () => {
  fetchData()
}

const handleScan = async () => {
  try {
    await ElMessageBox.confirm(
      '将按当前后端规则触发一次全库缺集扫描。任务会在后台执行，完成后自动刷新当前列表。',
      '启动扫描',
      {
        confirmButtonText: '开始扫描',
        cancelButtonText: '取消',
        type: 'warning',
        customClass: 'media-gap-scan-confirm',
        confirmButtonClass: 'btn-ember media-gap-scan-confirm__confirm',
        cancelButtonClass: 'media-gap-scan-confirm__cancel'
      }
    )
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      throw error
    }
    return
  }

  scanning.value = true
  try {
    const res = await scanMediaGaps()
    applyScanStatus({
      scanId: res.data?.scanId,
      scope: res.data?.scope,
      status: res.data?.status ?? 'running',
      running: res.data?.running ?? true,
      message: res.data?.message,
      count: res.data?.count
    })
    ElMessage.success(res.data?.message || '缺集扫描已启动，后台处理中')
    scheduleScanStatusPoll()
  } catch {
    // handled
  }
}

const applySearchResult = (result: MediaGapSearchResult, fallbackGap: MediaGapItem) => {
  candidateResult.value = {
    mediaGap: result.mediaGap ?? fallbackGap,
    candidates: result.candidates ?? [],
    searchedAt: result.searchedAt,
    source: result.source
  }
  selectedCandidateId.value = candidateResult.value.candidates[0]?.id ?? ''
  patchGap(result.mediaGap ?? fallbackGap)
}

const runSearch = async (gap: MediaGapItem) => {
  dialogLoading.value = true
  try {
    const res = await searchMediaGap(gap.id)
    const result = normalizeSearchResult(res.data, gap)
    applySearchResult(result, gap)

    if (result.candidates.length > 0) {
      ElMessage.success(`已获取 ${result.candidates.length} 个候选资源`)
    } else {
      ElMessage.warning('当前未搜索到可用候选')
    }
  } catch {
    candidateResult.value = {
      mediaGap: gap,
      candidates: []
    }
  } finally {
    dialogLoading.value = false
  }
}

const openSearchDialog = async (gap: MediaGapItem) => {
  currentGap.value = gap
  selectedGapId.value = gap.id
  dialogVisible.value = true
  candidateResult.value = {
    mediaGap: gap,
    candidates: []
  }
  selectedCandidateId.value = ''
  await runSearch(gap)
  await fetchData()
}

const openGroupedSearch = async (group: MediaGapGroupedSeries) => {
  const gap = resolveActiveGap(group)
  if (!gap || isTerminalStatus(gap.status)) return
  await openSearchDialog(gap)
}

const handleDialogSearch = async () => {
  if (!currentGap.value) return
  await runSearch(currentGap.value)
  await fetchData()
}

const handleDispatch = async () => {
  if (!currentGap.value || !selectedCandidate.value) {
    ElMessage.warning('请先选择一个候选资源')
    return
  }

  dispatching.value = true
  try {
    const res = await dispatchMediaGap(currentGap.value.id, {
      candidateId: selectedCandidate.value.id,
      candidate: selectedCandidate.value,
      candidatePayload: selectedCandidate.value.payload
    })
    patchGap(res.data?.mediaGap ?? currentGap.value)
    dialogVisible.value = false
    ElMessage.success(res.data?.message || '补货下发成功')
    await fetchData()
  } catch {
    // handled
  } finally {
    dispatching.value = false
  }
}

const handleIgnore = async (gap: MediaGapItem) => {
  let reason = ''
  try {
    const result = await ElMessageBox.prompt('可选填写忽略原因，后续排查误报时会更省事。', '忽略缺集工单', {
      confirmButtonText: '确认忽略',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：资源命名差异 / 数据源误报',
      inputValue: gap.ignoreReason || ''
    })
    reason = result.value?.trim() || ''
  } catch (error) {
    if (isMessageBoxCancel(error)) {
      return
    }
    throw error
  }

  try {
    const res = await ignoreMediaGap(gap.id, { reason: reason || undefined })
    patchGap(res.data?.mediaGap ?? {
      ...gap,
      status: 'IGNORED',
      ignoreReason: reason || undefined
    })
    ElMessage.success(res.data?.message || '工单已忽略')
    await fetchData()
  } catch {
    // handled
  }
}

const ignoreGroupedGap = async (group: MediaGapGroupedSeries) => {
  const gap = resolveActiveGap(group)
  if (!gap || gap.status === 'INGESTED' || gap.status === 'IGNORED') return
  await handleIgnore(gap)
}

const ignoreSeasonGroup = async (series: MediaGapGroupedSeries, seasonGroup: { season: number; gaps: MediaGapItem[] }) => {
  const targets = seasonGroup.gaps.filter((gap) => gap.status !== 'INGESTED' && gap.status !== 'IGNORED')
  if (targets.length === 0) {
    ElMessage.info('这一季当前没有可忽略的缺集')
    return
  }

  try {
    await ElMessageBox.confirm(
      `将忽略《${series.seriesName}》${formatSeasonCode(seasonGroup.season)} 的 ${targets.length} 条缺集工单。`,
      '整季忽略',
      {
        confirmButtonText: '确认忽略',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      throw error
    }
    return
  }

  try {
    for (const gap of targets) {
      const res = await ignoreMediaGap(gap.id, {
        reason: `${formatSeasonCode(seasonGroup.season)} 季整季忽略`
      })
      patchGap(res.data?.mediaGap ?? {
        ...gap,
        status: 'IGNORED',
        ignoreReason: `${formatSeasonCode(seasonGroup.season)} 季整季忽略`
      })
    }
    ElMessage.success(`已忽略 ${targets.length} 条 ${formatSeasonCode(seasonGroup.season)} 工单`)
    await fetchData()
  } catch {
    // handled by interceptor
  }
}

onMounted(() => {
  fetchData()
  void refreshScanStatus(false)
})

onBeforeUnmount(() => {
  clearScanStatusPoll()
})

watch(viewMode, () => {
  queryParams.value.page = 1
  if (!currentPageSizes.value.includes(queryParams.value.pageSize ?? 0)) {
    queryParams.value.pageSize = defaultPageSize.value
  }
  fetchData()
})

watch(sortMode, () => {
  if (viewMode.value !== 'grouped') return
  queryParams.value.page = 1
  fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="缺集管理"
      description="默认按剧聚合查看缺集断层，先找出哪部剧有问题，再逐集搜索、下发和核销。"
    >
      <template #actions>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <div class="inline-flex rounded-2xl border border-gray-200 bg-white p-1 shadow-sm">
            <button
              @click="viewMode = 'grouped'"
              class="view-toggle-btn"
              :class="{ 'view-toggle-btn-active': viewMode === 'grouped' }"
            >
              <el-icon><Grid /></el-icon>
              聚合视图
            </button>
            <button
              @click="viewMode = 'table'"
              class="view-toggle-btn"
              :class="{ 'view-toggle-btn-active': viewMode === 'table' }"
            >
              <el-icon><Collection /></el-icon>
              明细视图
            </button>
          </div>

          <button
            @click="fetchData"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            <el-icon><RefreshRight /></el-icon>
            刷新列表
          </button>
          <button
            @click="handleScan"
            :disabled="scanning"
            class="btn-ember inline-flex cursor-pointer items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-60"
          >
            <el-icon><Upload /></el-icon>
            {{ scanning ? '扫描中...' : '触发全库扫描' }}
          </button>
        </div>
      </template>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-4 2xl:grid-cols-[minmax(0,max-content)_auto] 2xl:items-end"
        content-class="grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-[320px_220px_minmax(320px,420px)] 2xl:items-end"
        actions-class="flex flex-wrap items-center justify-end gap-2"
      >
        <EmberSearchInput
          v-model="queryParams.keyword"
          label="关键词"
          aria-label="按剧名、TMDB ID 或 Emby ID 筛选"
          placeholder="剧名 / TMDB ID / Emby ID"
          :icon="Search"
          type="text"
          inputmode="text"
          @enter="handleSearch"
        />

        <EmberSelectField
          v-model="queryParams.status"
          label="状态"
          placeholder="全部状态"
          clearable
        >
          <el-option label="全部状态" value="" />
          <el-option
            v-for="option in statusOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </EmberSelectField>

        <EmberDateRangeField
          v-model="airDateRange"
          label="播出日期"
          type="daterange"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          value-format="YYYY-MM-DD"
          unlink-panels
          :icon="Calendar"
        />

        <template #actions>
          <button
            @click="handleReset"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            <el-icon><RefreshRight /></el-icon>
            重置
          </button>
          <button
            @click="handleSearch"
            class="btn-ember inline-flex cursor-pointer items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
          >
            <el-icon><Search /></el-icon>
            查询
          </button>
        </template>
      </EmberFilterPanel>

      <div class="mt-4 flex flex-col gap-3 border-t border-gray-100 pt-4 xl:flex-row xl:items-center xl:justify-between">
        <div class="flex flex-wrap gap-2">
          <span
            v-for="stat in compactStats"
            :key="stat.label"
            :class="compactStatClass(stat.tone)"
          >
            <span class="compact-stat-label">{{ stat.label }}</span>
            <span class="compact-stat-value">{{ stat.value }}</span>
          </span>
        </div>

        <div
          v-if="viewMode === 'grouped'"
          class="flex flex-wrap items-center gap-2"
        >
          <span class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">排序</span>
          <button
            v-for="option in sortOptions"
            :key="option.value"
            @click="sortMode = option.value"
            class="sort-chip"
            :class="{ 'sort-chip-active': sortMode === option.value }"
          >
            {{ option.label }}
          </button>
        </div>
      </div>
    </EmberPageHeaderCard>

    <div v-if="viewMode === 'grouped'" class="space-y-4">
      <div v-if="loading" class="grid grid-cols-1 gap-4 lg:grid-cols-2 2xl:grid-cols-3">
        <div
          v-for="index in 6"
          :key="index"
          class="h-[320px] animate-pulse rounded-[28px] border border-gray-100 bg-white shadow-sm"
        ></div>
      </div>

      <div v-else-if="groupedData.length > 0" class="grid grid-cols-1 gap-4 lg:grid-cols-2 2xl:grid-cols-3">
        <article
          v-for="series in groupedData"
          :key="series.key"
          class="series-card"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="truncate text-lg font-semibold text-gray-900">{{ series.seriesName || '未命名剧集' }}</h3>
              </div>
              <div class="flex flex-wrap gap-2 text-xs text-gray-500">
                <span>
                  共 <span class="series-total-highlight">{{ series.totalGaps }}</span> 条工单
                </span>
                <span v-if="series.tmdbId">TMDB <code class="inline-code">{{ series.tmdbId }}</code></span>
                <span v-if="series.embySeriesId">Emby <code class="inline-code">{{ series.embySeriesId }}</code></span>
              </div>
            </div>
            <div class="text-right text-xs text-gray-500">
              <div>最近变化</div>
              <div class="mt-1 font-medium text-gray-700">{{ formatDateTime(series.latestUpdatedAt) }}</div>
            </div>
          </div>

          <div class="mt-5 space-y-4">
            <section
              v-for="seasonGroup in visibleSeasonGroups(series)"
              :key="seasonGroup.season"
              class="season-block"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="flex items-center gap-2">
                  <span class="season-pill">{{ formatSeasonCode(seasonGroup.season) }}</span>
                  <span class="text-xs text-gray-500">缺 {{ seasonGroup.gaps.length }} 集</span>
                </div>
                <div class="flex flex-wrap items-center justify-end gap-2">
                  <span class="text-xs text-gray-400">
                    已下发 {{ seasonGroup.gaps.filter((gap) => gap.status === 'REQUESTED').length }}
                  </span>
                  <button
                    @click="ignoreSeasonGroup(series, seasonGroup)"
                    class="season-action-btn season-action-btn-muted"
                  >
                    忽略本季缺集
                  </button>
                </div>
              </div>

              <div class="mt-3.5 flex flex-wrap gap-2 md:mt-4">
                <button
                  v-for="gap in visibleSeasonGaps(series.key, seasonGroup)"
                  :key="gap.id"
                  @click="selectGap(gap)"
                  :class="episodeChipClass(gap)"
                >
                  <span>{{ `E${String(gap.episode).padStart(2, '0')}` }}</span>
                </button>
                <button
                  v-if="hiddenSeasonGapCount(series.key, seasonGroup) > 0"
                  @click="toggleSeasonExpanded(series.key, seasonGroup.season)"
                  class="episode-chip episode-chip-more"
                >
                  <span>+{{ hiddenSeasonGapCount(series.key, seasonGroup) }} 集</span>
                </button>
                <button
                  v-else-if="seasonGroup.gaps.length > 12"
                  @click="toggleSeasonExpanded(series.key, seasonGroup.season)"
                  class="episode-chip episode-chip-less"
                >
                  <span>收起</span>
                </button>
              </div>
            </section>

            <div
              v-if="actionableSeasonGroups(series).length === 0"
              class="rounded-2xl border border-dashed border-gray-200 bg-gray-50/70 px-4 py-5 text-sm text-gray-500"
            >
              当前没有待处理缺集，已收口到已忽略或已入库摘要。
            </div>

            <div
              v-if="hiddenSeasonGroupCount(series) > 0"
              class="series-expand-panel"
            >
              <div class="text-sm font-medium text-gray-600">
                还有 {{ hiddenSeasonGroupCount(series) }} 个季存在缺集，默认先收起，避免整张卡片被历史季撑爆。
              </div>
              <button
                @click="toggleSeriesExpanded(series.key)"
                class="series-expand-btn"
              >
                展开剩余季
              </button>
            </div>

            <div
              v-else-if="actionableSeasonGroups(series).length > 2"
              class="series-expand-panel"
            >
              <div class="text-sm font-medium text-gray-500">
                当前已展开全部 {{ actionableSeasonGroups(series).length }} 个缺集季。
              </div>
              <button
                @click="toggleSeriesExpanded(series.key)"
                class="series-expand-btn series-expand-btn-muted"
              >
                收起到默认视图
              </button>
            </div>
          </div>

          <div
            v-if="resolveActiveGap(series)"
            class="series-card-footer"
          >
            <div class="min-w-0 space-y-1">
              <div class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-500">当前选中</div>
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-semibold text-gray-900">{{ formatEpisodeCode(resolveActiveGap(series)!) }}</span>
                <el-tag :type="statusMeta[resolveActiveGap(series)!.status].type" effect="light" round>
                  {{ statusMeta[resolveActiveGap(series)!.status].label }}
                </el-tag>
                <span class="text-xs text-gray-500">播出 {{ formatDateOnly(resolveActiveGap(series)!.airDate) }}</span>
              </div>
              <div
                v-if="resolveActiveGap(series)!.status === 'IGNORED' && resolveActiveGap(series)!.ignoreReason"
                class="text-xs text-gray-500"
              >
                {{ resolveActiveGap(series)!.ignoreReason }}
              </div>
              <div
                v-if="resolveActiveGap(series)!.status === 'DISPATCH_FAILED' && resolveActiveGap(series)!.lastDispatchError"
                class="text-xs text-red-600"
                :title="resolveActiveGap(series)!.lastDispatchError ?? ''"
              >
                MoviePilot 下发失败：{{ resolveActiveGap(series)!.lastDispatchError }}
              </div>
            </div>

            <div class="flex flex-wrap items-center justify-end gap-2">
              <button
                @click="openGroupedSearch(series)"
                :disabled="isTerminalStatus(resolveActiveGap(series)!.status)"
                class="series-action-btn"
                :class="{ 'series-action-btn-disabled': isTerminalStatus(resolveActiveGap(series)!.status) }"
              >
                <el-icon><Search /></el-icon>
                搜索当前集
              </button>
              <button
                @click="ignoreGroupedGap(series)"
                :disabled="resolveActiveGap(series)!.status === 'INGESTED' || resolveActiveGap(series)!.status === 'IGNORED'"
                class="series-action-btn series-action-btn-muted"
                :class="{ 'series-action-btn-disabled': resolveActiveGap(series)!.status === 'INGESTED' || resolveActiveGap(series)!.status === 'IGNORED' }"
              >
                <el-icon><InfoFilled /></el-icon>
                忽略当前集
              </button>
            </div>
          </div>
        </article>
      </div>

      <div
        v-else
        class="rounded-[28px] border border-dashed border-gray-200 bg-white px-6 py-16 text-center shadow-sm"
      >
        <el-icon class="mb-4 text-4xl text-gray-300"><Grid /></el-icon>
        <div class="text-lg font-semibold text-gray-800">当前没有可展示的缺集剧集</div>
        <div class="mt-2 text-sm text-gray-500">
          可以先触发全库扫描，或者调整筛选条件后再看。
        </div>
      </div>

      <div class="rounded-3xl border border-gray-100 bg-white px-4 py-3 shadow-sm">
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="currentPageSizes"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
        />
      </div>
    </div>

    <EmberTableCard
      v-else
      :data="tableData"
      :loading="loading"
      row-key="id"
      empty-text="暂无缺集工单"
    >
      <el-table-column prop="seriesName" label="剧集" min-width="280">
        <template #default="{ row }">
          <div class="space-y-1">
            <div class="font-medium text-gray-900">{{ row.seriesName || '-' }}</div>
            <div class="flex flex-wrap gap-2 text-xs text-gray-500">
              <span>{{ formatEpisodeCode(row) }}</span>
              <span v-if="row.tmdbId">TMDB: <code class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-700">{{ row.tmdbId }}</code></span>
              <span v-if="row.embySeriesId">Emby: <code class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-700">{{ row.embySeriesId }}</code></span>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="airDate" label="播出日期" width="140">
        <template #default="{ row }">
          {{ formatDateOnly(row.airDate) }}
        </template>
      </el-table-column>

      <el-table-column prop="status" label="状态" width="140">
        <template #default="{ row }">
          <div class="space-y-1">
            <el-tag :type="statusMeta[row.status].type" effect="light" round>
              {{ statusMeta[row.status].label }}
            </el-tag>
            <div v-if="row.status === 'IGNORED' && row.ignoreReason" class="text-xs leading-5 text-gray-500">
              {{ row.ignoreReason }}
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="扫描 / 搜索" min-width="190">
        <template #default="{ row }">
          <div class="space-y-1 text-sm text-gray-600">
            <div>扫描：{{ formatDateTime(row.lastScannedAt) }}</div>
            <div>搜索：{{ formatDateTime(row.lastSearchedAt) }}</div>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="下发 / 入库" min-width="190">
        <template #default="{ row }">
          <div class="space-y-1 text-sm text-gray-600">
            <div>下发：{{ formatDateTime(row.requestedAt) }}</div>
            <div>入库：{{ formatDateTime(row.ingestedAt) }}</div>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="updatedAt" label="更新时间" width="170">
        <template #default="{ row }">
          {{ formatDateTime(row.updatedAt) }}
        </template>
      </el-table-column>

      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
            <button
              @click="openSearchDialog(row)"
              :disabled="isTerminalStatus(row.status)"
              class="action-link"
              :class="{ 'action-link--disabled': isTerminalStatus(row.status) }"
            >
              搜索
            </button>
            <button
              @click="handleIgnore(row)"
              :disabled="row.status === 'INGESTED' || row.status === 'IGNORED'"
              class="action-link action-link--muted"
              :class="{ 'action-link--disabled': row.status === 'INGESTED' || row.status === 'IGNORED' }"
            >
              忽略
            </button>
          </div>
        </template>
      </el-table-column>

      <template #pagination>
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="currentPageSizes"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
        />
      </template>
    </EmberTableCard>

    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="760px"
      destroy-on-close
      align-center
    >
      <div class="space-y-4">
        <div class="rounded-2xl border border-gray-100 bg-gray-50/80 p-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="space-y-1">
              <div class="flex items-center gap-2 text-base font-semibold text-gray-900">
                <span>{{ currentGap?.seriesName || '-' }}</span>
                <el-tag v-if="currentGap" :type="statusMeta[currentGap.status].type" effect="light" round>
                  {{ statusMeta[currentGap.status].label }}
                </el-tag>
              </div>
              <div class="flex flex-wrap gap-3 text-sm text-gray-600">
                <span v-if="currentGap">{{ formatEpisodeCode(currentGap) }}</span>
                <span>播出日期：{{ formatDateOnly(currentGap?.airDate) }}</span>
                <span v-if="candidateResult.source">来源：{{ candidateResult.source }}</span>
              </div>
            </div>

            <button
              @click="handleDialogSearch"
              :disabled="dialogLoading"
              class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <el-icon><Search /></el-icon>
              {{ dialogLoading ? '搜索中...' : '重新搜索' }}
            </button>
          </div>
        </div>

        <div v-if="dialogLoading" class="space-y-4">
          <div class="rounded-2xl border border-amber-100 bg-amber-50/80 px-4 py-3 text-sm text-amber-700">
            <div class="flex items-center gap-2 font-semibold">
              <el-icon class="animate-spin"><Loading /></el-icon>
              <span>正在搜索候选资源</span>
            </div>
            <div class="mt-1 text-xs text-amber-600">
              正在向资源源拉取当前剧集的候选列表，通常需要几秒，请稍等。
            </div>
          </div>

          <div class="space-y-3">
            <div class="h-20 animate-pulse rounded-2xl bg-gray-100"></div>
            <div class="h-20 animate-pulse rounded-2xl bg-gray-100"></div>
            <div class="h-20 animate-pulse rounded-2xl bg-gray-100"></div>
          </div>
        </div>

        <div v-else-if="candidateResult.candidates.length > 0" class="space-y-3">
          <div class="flex items-center justify-between text-sm text-gray-500">
            <span>共找到 {{ candidateResult.candidates.length }} 个候选</span>
            <span v-if="candidateResult.searchedAt">搜索时间：{{ formatDateTime(candidateResult.searchedAt) }}</span>
          </div>

          <div class="max-h-[420px] space-y-3 overflow-y-auto pr-1">
            <div
              v-for="candidate in candidateResult.candidates"
              :key="candidate.id"
              class="candidate-card"
              :class="{ 'candidate-card--active': candidate.id === selectedCandidateId }"
              @click="selectedCandidateId = candidate.id"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="space-y-2">
                  <div class="flex items-start gap-2">
                    <el-icon
                      class="mt-0.5 shrink-0"
                      :class="candidate.id === selectedCandidateId ? 'text-ember' : 'text-gray-300'"
                    >
                      <CircleCheckFilled />
                    </el-icon>
                    <div class="space-y-1">
                      <div class="font-medium text-gray-900">{{ candidate.title }}</div>
                      <div v-if="candidate.subtitle" class="text-sm text-gray-500">
                        {{ candidate.subtitle }}
                      </div>
                    </div>
                  </div>

                  <div class="flex flex-wrap gap-2">
                    <span v-if="candidate.source" class="candidate-chip">{{ candidate.source }}</span>
                    <span v-if="candidate.site" class="candidate-chip">{{ candidate.site }}</span>
                    <span v-if="candidate.size" class="candidate-chip">{{ candidate.size }}</span>
                    <span v-if="candidate.seeders !== undefined" class="candidate-chip">做种 {{ candidate.seeders }}</span>
                    <span v-if="candidate.language" class="candidate-chip">{{ candidate.language }}</span>
                    <span v-if="candidate.releaseGroup" class="candidate-chip">{{ candidate.releaseGroup }}</span>
                    <span v-if="candidate.episodeRange" class="candidate-chip">{{ candidate.episodeRange }}</span>
                  </div>

                  <div v-if="candidate.matchReason || candidate.description" class="space-y-1 text-sm text-gray-600">
                    <div v-if="candidate.matchReason" class="flex items-start gap-1.5">
                      <el-icon class="mt-0.5 text-amber-500"><InfoFilled /></el-icon>
                      <span>{{ candidate.matchReason }}</span>
                    </div>
                    <div v-if="candidate.description" class="candidate-description">
                      {{ candidate.description }}
                    </div>
                  </div>
                </div>

                <div class="shrink-0 text-right text-xs text-gray-500">
                  <div class="inline-flex items-center gap-1">
                    <el-icon><Clock /></el-icon>
                    <span>{{ formatDateTime(candidate.publishDate) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-else
          class="rounded-2xl border border-dashed border-gray-200 bg-gray-50/70 px-6 py-10 text-center"
        >
          <el-icon class="mb-3 text-3xl text-gray-300"><Download /></el-icon>
          <div class="text-base font-medium text-gray-700">当前没有可用候选</div>
          <div class="mt-1 text-sm text-gray-500">
            可以稍后重试搜索，或先检查后端搜索条件与资源源配置。
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <button
            @click="dialogVisible = false"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100"
          >
            关闭
          </button>
          <button
            @click="handleDispatch"
            :disabled="!canDispatch"
            class="btn-ember inline-flex cursor-pointer items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-60"
          >
            <el-icon><CircleCheckFilled /></el-icon>
            {{ dispatching ? '下发中...' : '确认下发' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.view-toggle-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  border-radius: 1rem;
  padding: 0.625rem 0.875rem;
  font-size: 0.875rem;
  color: #4b5563;
  transition:
    background-color 0.2s ease,
    color 0.2s ease,
    box-shadow 0.2s ease;
}

.view-toggle-btn:hover {
  background: #f3f4f6;
  color: #111827;
}

.view-toggle-btn-active {
  background: rgba(229, 9, 20, 0.08);
  color: var(--ember-red);
  box-shadow: 0 8px 18px rgba(229, 9, 20, 0.08);
}

.sort-chip {
  border-radius: 9999px;
  border: 1px solid #e5e7eb;
  background: #fff;
  padding: 0.45rem 0.8rem;
  font-size: 0.75rem;
  font-weight: 700;
  color: #4b5563;
  transition:
    color 0.18s ease,
    border-color 0.18s ease,
    background-color 0.18s ease,
    transform 0.18s ease;
}

.sort-chip:hover {
  transform: translateY(-1px);
  border-color: #cbd5e1;
  color: #111827;
}

.sort-chip-active {
  border-color: rgba(229, 9, 20, 0.24);
  background: rgba(229, 9, 20, 0.08);
  color: var(--ember-red);
}

.compact-stat {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  border-radius: 9999px;
  padding: 0.45rem 0.78rem;
  font-size: 0.75rem;
  font-weight: 700;
  border: 1px solid transparent;
}

.compact-stat-neutral {
  background: #f8fafc;
  border-color: #e2e8f0;
  color: #475569;
}

.compact-stat-missing {
  background: #fef2f2;
  border-color: #fecaca;
  color: #dc2626;
}

.compact-stat-requested {
  background: #eff6ff;
  border-color: #bfdbfe;
  color: #2563eb;
}

.compact-stat-settled {
  background: #ecfdf5;
  border-color: #a7f3d0;
  color: #059669;
}

.compact-stat-muted {
  background: #f3f4f6;
  border-color: #d1d5db;
  color: #6b7280;
}

.compact-stat-label {
  opacity: 0.86;
}

.compact-stat-value {
  color: #111827;
}

.series-card {
  border: 1px solid #eef2f7;
  border-radius: 1.75rem;
  background:
    linear-gradient(180deg, rgba(248, 250, 252, 0.92), rgba(255, 255, 255, 1)),
    #fff;
  padding: 1.25rem;
  box-shadow: 0 16px 30px rgba(15, 23, 42, 0.06);
}

.series-total-highlight {
  color: var(--ember-red);
  font-weight: 700;
}

.season-pill {
  display: inline-flex;
  align-items: center;
  border-radius: 9999px;
  background: #111827;
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  font-weight: 700;
  color: #fff;
}

.season-block {
  border-radius: 1rem;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.78), rgba(255, 255, 255, 0.98));
  padding: 0.85rem;
}

.season-action-btn {
  border-radius: 9999px;
  border: 1px solid #e5e7eb;
  background: #fff;
  padding: 0.35rem 0.7rem;
  font-size: 0.7rem;
  font-weight: 700;
  color: #4b5563;
  transition:
    background-color 0.18s ease,
    color 0.18s ease,
    border-color 0.18s ease,
    transform 0.18s ease;
}

.season-action-btn:hover {
  transform: translateY(-1px);
  border-color: #cbd5e1;
}

.season-action-btn-primary {
  border-color: rgba(37, 99, 235, 0.16);
  background: rgba(37, 99, 235, 0.08);
  color: #2563eb;
}

.season-action-btn-muted {
  border-color: rgba(107, 114, 128, 0.16);
  background: #f8fafc;
  color: #6b7280;
}

.episode-chip {
  border: 1px solid transparent;
  border-radius: 9999px;
  padding: 0.48rem 0.78rem;
  font-size: 0.75rem;
  font-weight: 700;
  transition:
    transform 0.18s ease,
    box-shadow 0.18s ease,
    border-color 0.18s ease,
    background-color 0.18s ease;
}

.episode-chip:hover {
  transform: translateY(-1px);
}

.episode-chip-selected {
  box-shadow: 0 0 0 2px rgba(229, 9, 20, 0.12);
}

.episode-chip-missing {
  background: #fef2f2;
  border-color: #fecaca;
  color: #dc2626;
}

.episode-chip-searched {
  background: #fff7ed;
  border-color: #fed7aa;
  color: #d97706;
}

.episode-chip-requested {
  background: #eff6ff;
  border-color: #bfdbfe;
  color: #2563eb;
}

.episode-chip-dispatch-failed {
  background: #fef2f2;
  border-color: #fca5a5;
  color: #b91c1c;
}

.episode-chip-ingested {
  background: #ecfdf5;
  border-color: #a7f3d0;
  color: #059669;
}

.episode-chip-ignored {
  background: #f3f4f6;
  border-color: #d1d5db;
  color: #6b7280;
}

.episode-chip-more {
  background: #fff;
  border-color: #cbd5e1;
  color: #334155;
}

.episode-chip-less {
  background: #fffaf0;
  border-color: #fcd34d;
  color: #b45309;
}

.series-card-footer {
  margin-top: 1.25rem;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border-top: 1px solid #f1f5f9;
  padding-top: 1rem;
}

.series-expand-panel {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border-top: 1px dashed #e2e8f0;
  padding-top: 0.9rem;
}

.series-expand-btn {
  border-radius: 9999px;
  background: rgba(229, 9, 20, 0.08);
  padding: 0.5rem 0.85rem;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--ember-red);
  transition:
    transform 0.18s ease,
    background-color 0.18s ease;
}

.series-expand-btn:hover {
  transform: translateY(-1px);
  background: rgba(229, 9, 20, 0.12);
}

.series-expand-btn-muted {
  background: #f3f4f6;
  color: #4b5563;
}

.series-action-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  border-radius: 9999px;
  background: rgba(229, 9, 20, 0.08);
  padding: 0.55rem 0.9rem;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--ember-red);
  transition:
    transform 0.18s ease,
    opacity 0.18s ease,
    background-color 0.18s ease;
}

.series-action-btn:hover {
  transform: translateY(-1px);
  background: rgba(229, 9, 20, 0.12);
}

.series-action-btn-muted {
  background: #f3f4f6;
  color: #4b5563;
}

.series-action-btn-disabled {
  cursor: not-allowed;
  opacity: 0.45;
  pointer-events: none;
}

.inline-code {
  border-radius: 9999px;
  background: #f3f4f6;
  padding: 0.15rem 0.45rem;
  font-size: 0.7rem;
  color: #374151;
}

.action-link {
  color: var(--ember-red);
  cursor: pointer;
  font-size: 0.875rem;
  font-weight: 500;
  transition: color 0.2s ease;
}

.action-link:hover {
  color: rgba(229, 9, 20, 0.78);
}

.action-link--muted {
  color: #6b7280;
}

.action-link--disabled {
  color: #9ca3af;
  cursor: not-allowed;
  pointer-events: none;
}

.candidate-card {
  cursor: pointer;
  border: 1px solid #e5e7eb;
  border-radius: 1rem;
  background: #ffffff;
  padding: 1rem;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease,
    transform 0.2s ease;
}

.candidate-card:hover {
  border-color: rgba(229, 9, 20, 0.18);
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.08);
  transform: translateY(-1px);
}

.candidate-card--active {
  border-color: rgba(229, 9, 20, 0.55);
  box-shadow:
    0 0 0 1px rgba(229, 9, 20, 0.18),
    0 12px 28px rgba(229, 9, 20, 0.08);
}

.candidate-chip {
  border-radius: 9999px;
  background: #f3f4f6;
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  color: #4b5563;
}

.candidate-description {
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  line-height: 1.5;
}

:deep(.media-gap-scan-confirm) {
  border-radius: 1.5rem;
  padding: 0.5rem;
}

:deep(.media-gap-scan-confirm .el-message-box__btns) {
  padding-top: 0.5rem;
}

:deep(.media-gap-scan-confirm__confirm),
:deep(.media-gap-scan-confirm__cancel) {
  min-height: 42px !important;
  border-radius: 0.75rem !important;
  padding: 0 1rem !important;
  font-size: 0.875rem !important;
  font-weight: 600 !important;
  box-shadow: none !important;
}

:deep(.media-gap-scan-confirm__confirm.el-button) {
  border: none !important;
}

:deep(.media-gap-scan-confirm__cancel.el-button) {
  border: 1px solid #e5e7eb !important;
  background: #ffffff !important;
  color: #374151 !important;
}

:deep(.media-gap-scan-confirm__cancel.el-button:hover) {
  border-color: #d1d5db !important;
  background: #f9fafb !important;
  color: #111827 !important;
}

:deep(.media-gap-scan-confirm .el-message-box__btns .el-button) {
  border-radius: 0.75rem !important;
}
</style>
