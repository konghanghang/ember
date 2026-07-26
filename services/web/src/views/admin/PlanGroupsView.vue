<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  CollectionTag,
  Delete,
  EditPen,
  FolderOpened,
  Plus,
  Refresh,
  Setting
} from '@element-plus/icons-vue'
import {
  createPlanGroup,
  deletePlanGroup,
  getAdminMediaLibraries,
  getEmbyPolicySyncBatch,
  getPlanGroupEmbyPolicyTemplate,
  getPlanGroupMediaLibraries,
  getPlanGroups,
  retryFailedEmbyPolicySyncBatch,
  updatePlanGroup,
  updatePlanGroupEmbyPolicyTemplate,
  updatePlanGroupMediaLibraries
} from '@/api/admin'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import { isConflictError } from '@/utils/api-error'
import { resolveGroupPolicySyncPresentation } from '@/utils/policy-sync'
import { formatMediaLibrarySummary } from '@/utils/media-library'
import type {
  CreatePlanGroupRequest,
  EmbyPolicySyncBatchCreated,
  EmbyPolicySyncBatchDetail,
  EmbyPolicySyncStatus,
  ManagedPlanGroup,
  MediaLibraryOption,
  PlanGroupEmbyPolicyTemplateUpdateRequest,
  PlanGroupMediaLibraryUpdateResult,
  UpdatePlanGroupRequest
} from '@/types/api'

const route = useRoute()
const loading = ref(false)
const creating = ref(false)
const updating = ref(false)
const savingLibraries = ref(false)
const savingLibrariesMode = ref<'deferred' | 'batch' | null>(null)
const savingPolicy = ref(false)
const loadingTemplate = ref(false)
const retryingSyncBatch = ref(false)
const groups = ref<ManagedPlanGroup[]>([])
const activeSyncBatch = ref<EmbyPolicySyncBatchDetail | null>(null)
const syncPollingTimer = ref<number | null>(null)
const syncTerminalNotified = ref(false)
const syncPollErrorCount = ref(0)
// 轮询连续失败上限：瞬时错误不终止轮询，但连续失败超过该值后停止，避免后端长期不可用时静默空转。
const SYNC_POLL_MAX_ERRORS = 5

const dialogVisible = ref(false)
const editDialogVisible = ref(false)
const mediaDialogVisible = ref(false)
const policyDialogVisible = ref(false)
const selectedGroup = ref<ManagedPlanGroup | null>(null)
const allLibraries = ref<MediaLibraryOption[]>([])
const selectedLibraryIds = ref<string[]>([])

const createForm = ref({
  key: '',
  name: '',
  description: '',
  isDefault: false,
  sortOrder: 0,
  subscriptionAutoApproveDailyLimit: 0
})

const editForm = ref({
  key: '',
  name: '',
  description: '',
  isDefault: false,
  sortOrder: 0,
  subscriptionAutoApproveDailyLimit: 0
})

const policyForm = ref<PlanGroupEmbyPolicyTemplateUpdateRequest>({
  simultaneousStreamLimit: 3,
  enableContentDownloading: false,
  enableLiveTvAccess: false,
  enableSyncTranscoding: false,
  enableAudioPlaybackTranscoding: false,
  enableVideoPlaybackTranscoding: false,
  enablePlaybackRemuxing: true,
  enableRemoteAccess: true
})

const defaultGroup = computed(() => groups.value.find(group => group.isDefault) ?? null)
const referenceCount = computed(() => groups.value.reduce((sum, item) => sum + (item.planCount ?? 0) + (item.userCount ?? 0), 0))
const mediaLibraryTemplateCount = computed(() => groups.value.reduce((sum, item) => sum + (item.mediaLibraryCount ?? 0), 0))
const activeSyncCompletedCount = computed(() => {
  if (!activeSyncBatch.value) return 0
  return activeSyncBatch.value.syncedCount + activeSyncBatch.value.failedCount
})
const activeSyncProgress = computed(() => {
  if (!activeSyncBatch.value || activeSyncBatch.value.totalCount <= 0) return 100
  return Math.round((activeSyncCompletedCount.value / activeSyncBatch.value.totalCount) * 100)
})
const canRetryActiveSyncBatch = computed(() => {
  const batch = activeSyncBatch.value
  return !!batch
    && batch.failedCount > 0
    && (batch.status === 'failed' || batch.status === 'partial_failed')
})
const activeSyncFailedUsers = computed(() => (activeSyncBatch.value?.failedUsers ?? []).slice(0, 5))
const activeSyncHiddenFailedUserCount = computed(() => {
  const total = activeSyncBatch.value?.failedCount ?? 0
  return Math.max(total - activeSyncFailedUsers.value.length, 0)
})

/** 判断分组是否允许删除；默认分组是系统兜底分组，不能暴露删除入口。 */
const canDeletePlanGroup = (group: ManagedPlanGroup) => !group.isDefault

/** 判断同步批次是否已进入终态，决定短轮询何时停止。 */
const isTerminalSyncStatus = (status: EmbyPolicySyncStatus) => {
  return status === 'synced' || status === 'partial_failed' || status === 'failed' || status === 'out_of_sync'
}

/** 把模板保存后返回的同步批次压缩成管理员可执行的结果反馈。 */
const showSyncBatchResult = (batch: EmbyPolicySyncBatchCreated) => {
  if (batch.affectedUserCount > 0) {
    ElMessage.success(`模板已保存，已创建 ${batch.affectedUserCount} 个用户同步任务`)
    return
  }
  ElMessage.success('模板已保存')
}

const showMediaLibraryUpdateResult = (result: PlanGroupMediaLibraryUpdateResult) => {
  if (result.mode === 'deferred') {
    if ((result.outOfSyncUserCount ?? 0) > 0) {
      ElMessage.success(`模板已保存，${result.outOfSyncUserCount} 个用户待同步`)
      return
    }
    ElMessage.success('模板已保存')
    return
  }
  showSyncBatchResult({
    batchId: result.batchId ?? '',
    affectedUserCount: result.affectedUserCount,
    status: result.status,
  })
}

const stopSyncBatchPolling = () => {
  if (syncPollingTimer.value !== null) {
    window.clearInterval(syncPollingTimer.value)
    syncPollingTimer.value = null
  }
}

const syncBatchIdFromRoute = () => {
  const value = route.query.syncBatchId
  return typeof value === 'string' ? value.trim() : ''
}

/** 从用户列表等入口跳转过来时，直接展示指定同步批次详情。 */
const loadSyncBatchFromRoute = async () => {
  const batchId = syncBatchIdFromRoute()
  if (!batchId) return

  stopSyncBatchPolling()
  syncTerminalNotified.value = true
  syncPollErrorCount.value = 0
  try {
    const res = await getEmbyPolicySyncBatch(batchId)
    activeSyncBatch.value = res.data
    if (!isTerminalSyncStatus(res.data.status)) {
      syncTerminalNotified.value = false
      syncPollingTimer.value = window.setInterval(() => {
        void pollSyncBatch(batchId)
      }, 2500)
    }
  } catch {
    // request interceptor 已提示错误，页面保持当前列表可用。
  }
}

/** 根据 batch id 轮询同步进度，终态后刷新分组摘要。
 *  瞬时错误（网络抖动、后端短暂 5xx）只累计错误计数，不终止轮询；
 *  连续失败超过 SYNC_POLL_MAX_ERRORS 才停止，避免后端长期不可达时空转。
 */
const pollSyncBatch = async (batchId: string) => {
  try {
    const res = await getEmbyPolicySyncBatch(batchId)
    activeSyncBatch.value = res.data
    syncPollErrorCount.value = 0
    if (!isTerminalSyncStatus(res.data.status)) return

    stopSyncBatchPolling()
    await fetchData()
    if (syncTerminalNotified.value) return

    syncTerminalNotified.value = true
    if (res.data.status === 'synced') {
      ElMessage.success('用户同步已完成')
    } else if (res.data.status === 'partial_failed') {
      ElMessage.warning(`用户同步部分失败：${res.data.failedCount} 个失败`)
    } else {
      ElMessage.error('用户同步失败')
    }
  } catch {
    syncPollErrorCount.value += 1
    if (syncPollErrorCount.value >= SYNC_POLL_MAX_ERRORS) {
      // 连续多次失败视为批次不可达，停止轮询；request 拦截器已负责每次错误提示。
      stopSyncBatchPolling()
    }
  }
}

/** 启动保存模板后的短轮询；真实同步由后端 worker 执行。 */
const startSyncBatchPolling = (batch: EmbyPolicySyncBatchCreated) => {
  stopSyncBatchPolling()
  syncTerminalNotified.value = false
  syncPollErrorCount.value = 0
  if (batch.affectedUserCount <= 0) {
    activeSyncBatch.value = null
    return
  }
  void pollSyncBatch(batch.batchId)
  syncPollingTimer.value = window.setInterval(() => {
    void pollSyncBatch(batch.batchId)
  }, 2500)
}

const handleMediaLibraryUpdateResult = async (result: PlanGroupMediaLibraryUpdateResult) => {
  showMediaLibraryUpdateResult(result)
  if (result.mode === 'batch' && result.batchId) {
    startSyncBatchPolling({
      batchId: result.batchId,
      affectedUserCount: result.affectedUserCount,
      status: result.status,
    })
  } else {
    stopSyncBatchPolling()
    activeSyncBatch.value = null
  }
  await fetchData()
}

/** 对失败的 Emby Policy 批次创建补偿重试批次，并继续展示新批次进度。 */
const handleRetryFailedSyncBatch = async () => {
  if (!activeSyncBatch.value || !canRetryActiveSyncBatch.value) return

  retryingSyncBatch.value = true
  try {
    const res = await retryFailedEmbyPolicySyncBatch(activeSyncBatch.value.id)
    if (res.data.affectedUserCount <= 0) {
      ElMessage.info('没有可重试的失败项')
      await fetchData()
      return
    }

    ElMessage.success(`已提交 ${res.data.affectedUserCount} 个失败项重试`)
    startSyncBatchPolling(res.data)
    await fetchData()
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('存在未完成同步任务，稍后再重试')
    }
  } finally {
    retryingSyncBatch.value = false
  }
}

/** 从后端模板详情中抽取已选媒体库 ID，避免依赖列表接口的扩展字段。 */
const resolveSelectedLibraryIds = (libraries: MediaLibraryOption[]) => {
  return libraries.map(item => item.id).filter(Boolean)
}

/** 拉取分组列表及摘要字段，作为本页面所有模板入口的基础事实。 */
const fetchData = async () => {
  loading.value = true
  try {
    const res = await getPlanGroups()
    groups.value = res.data ?? []
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.value = {
    key: '',
    name: '',
    description: '',
    isDefault: false,
    sortOrder: 0,
    subscriptionAutoApproveDailyLimit: 0
  }
}

const openEditDialog = (group: ManagedPlanGroup) => {
  editForm.value = {
    key: group.key,
    name: group.name,
    description: group.description ?? '',
    isDefault: group.isDefault,
    sortOrder: group.sortOrder,
    subscriptionAutoApproveDailyLimit: group.subscriptionAutoApproveDailyLimit ?? 0
  }
  editDialogVisible.value = true
}

/** 打开媒体库模板弹窗时同时读取 Emby 当前库和该分组已保存模板。 */
const openMediaDialog = async (group: ManagedPlanGroup) => {
  selectedGroup.value = group
  mediaDialogVisible.value = true
  loadingTemplate.value = true
  selectedLibraryIds.value = []

  try {
    const [libraryRes, settingRes] = await Promise.all([
      getAdminMediaLibraries(),
      getPlanGroupMediaLibraries(group.key)
    ])
    allLibraries.value = libraryRes.data ?? []
    selectedLibraryIds.value = resolveSelectedLibraryIds(settingRes.data?.libraries ?? [])
  } catch {
    // request interceptor 已负责错误提示；弹窗保留，允许管理员重试。
  } finally {
    loadingTemplate.value = false
  }
}

/** 打开权益模板弹窗时只加载首版托管字段，不把 Emby 全量 Policy 暴露到前端。 */
const openPolicyDialog = async (group: ManagedPlanGroup) => {
  selectedGroup.value = group
  policyDialogVisible.value = true
  loadingTemplate.value = true

  try {
    const res = await getPlanGroupEmbyPolicyTemplate(group.key)
    const template = res.data
    policyForm.value = {
      simultaneousStreamLimit: template.simultaneousStreamLimit,
      enableContentDownloading: template.enableContentDownloading,
      enableLiveTvAccess: template.enableLiveTvAccess,
      enableSyncTranscoding: template.enableSyncTranscoding,
      enableAudioPlaybackTranscoding: template.enableAudioPlaybackTranscoding,
      enableVideoPlaybackTranscoding: template.enableVideoPlaybackTranscoding,
      enablePlaybackRemuxing: template.enablePlaybackRemuxing,
      enableRemoteAccess: template.enableRemoteAccess
    }
  } catch {
    // request interceptor 已负责错误提示；保留默认值避免表单残缺。
  } finally {
    loadingTemplate.value = false
  }
}

const handleCreate = async () => {
  if (!createForm.value.key.trim() || !createForm.value.name.trim()) {
    ElMessage.warning('请填写分组标识和分组名称')
    return
  }

  const payload: CreatePlanGroupRequest = {
    key: createForm.value.key.trim(),
    name: createForm.value.name.trim(),
    description: createForm.value.description.trim(),
    isDefault: createForm.value.isDefault,
    sortOrder: createForm.value.sortOrder,
    subscriptionAutoApproveDailyLimit: createForm.value.subscriptionAutoApproveDailyLimit
  }

  creating.value = true
  try {
    await createPlanGroup(payload)
    ElMessage.success('分组创建成功')
    dialogVisible.value = false
    resetCreateForm()
    await fetchData()
  } finally {
    creating.value = false
  }
}

const handleUpdate = async () => {
  if (!editForm.value.name.trim()) {
    ElMessage.warning('请输入分组名称')
    return
  }

  const payload: UpdatePlanGroupRequest = {
    name: editForm.value.name.trim(),
    description: editForm.value.description.trim(),
    isDefault: editForm.value.isDefault,
    sortOrder: editForm.value.sortOrder,
    subscriptionAutoApproveDailyLimit: editForm.value.subscriptionAutoApproveDailyLimit
  }

  updating.value = true
  try {
    await updatePlanGroup(editForm.value.key, payload)
    ElMessage.success('分组更新成功')
    editDialogVisible.value = false
    await fetchData()
  } finally {
    updating.value = false
  }
}

const handleDelete = async (group: ManagedPlanGroup) => {
  if (!canDeletePlanGroup(group)) {
    ElMessage.warning('默认分组不能删除')
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定删除分组 ${group.name} 吗？若仍有用户或套餐引用，后端会直接拒绝。`,
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger'
      }
    )

    await deletePlanGroup(group.key)
    ElMessage.success('分组删除成功')
    await fetchData()
  } catch {
    // cancelled or handled by request interceptor
  }
}

/** 保存分组媒体库模板，并把 409 同步中状态转成明确的页面级反馈。 */
const handleSaveMediaLibraries = async (applyToExistingUsers: boolean = true) => {
  if (!selectedGroup.value) return

  savingLibraries.value = true
  savingLibrariesMode.value = applyToExistingUsers ? 'batch' : 'deferred'
  try {
    const res = await updatePlanGroupMediaLibraries(selectedGroup.value.key, selectedLibraryIds.value, applyToExistingUsers)
    mediaDialogVisible.value = false
    await handleMediaLibraryUpdateResult(res.data)
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('该分组有同步任务未完成，稍后再保存')
    }
  } finally {
    savingLibraries.value = false
    savingLibrariesMode.value = null
  }
}

/** 保存 Emby 权益模板；后端负责校验托管字段范围和创建同步批次。 */
const handleSavePolicyTemplate = async () => {
  if (!selectedGroup.value) return

  savingPolicy.value = true
  try {
    const res = await updatePlanGroupEmbyPolicyTemplate(selectedGroup.value.key, policyForm.value)
    policyDialogVisible.value = false
    showSyncBatchResult(res.data)
    startSyncBatchPolling(res.data)
    await fetchData()
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('该分组有同步任务未完成，稍后再保存')
    }
  } finally {
    savingPolicy.value = false
  }
}

watch(
  () => route.query.syncBatchId,
  () => {
    void loadSyncBatchFromRoute()
  }
)

onMounted(async () => {
  await fetchData()
  await loadSyncBatchFromRoute()
})
onBeforeUnmount(stopSyncBatchPolling)
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="用户分组 / 权益模板"
      description="用户分组、媒体库模板和 Emby 权益模板统一在这里维护。"
    >
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">{{ groups.length }} 个分组</span>
      </template>

      <template #actions>
        <div class="flex items-center gap-3">
          <button
            @click="fetchData"
            class="inline-flex h-11 w-11 items-center justify-center cursor-pointer rounded-xl border border-gray-200 bg-white text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
            aria-label="刷新用户分组列表"
          >
            <el-icon :size="20"><Refresh /></el-icon>
          </button>
          <button
            @click="dialogVisible = true"
            class="btn-ember inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer"
          >
            <el-icon><Plus /></el-icon>
            <span>新建分组</span>
          </button>
        </div>
      </template>
    </EmberPageHeaderCard>

    <div class="grid gap-4 lg:grid-cols-3">
      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <div class="text-sm font-semibold text-gray-500">默认分组</div>
        <div class="mt-3 flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-amber-50 text-amber-500">
            <el-icon :size="22"><CollectionTag /></el-icon>
          </div>
          <div>
            <div class="text-lg font-semibold text-gray-900">{{ defaultGroup?.name || '未设置' }}</div>
            <div class="text-sm text-gray-500">{{ defaultGroup?.key || '暂无默认分组' }}</div>
          </div>
        </div>
      </div>

      <EmberMetricCard title="媒体库模板" :value="mediaLibraryTemplateCount" />

      <EmberMetricCard title="分组引用总量" :value="referenceCount" />
    </div>

    <div
      v-if="activeSyncBatch"
      class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm"
    >
      <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <span class="text-sm font-semibold text-gray-900">Emby Policy 同步</span>
            <el-tag :type="resolveGroupPolicySyncPresentation(activeSyncBatch.status).tagType" effect="light" round size="small">
              {{ resolveGroupPolicySyncPresentation(activeSyncBatch.status).label }}
            </el-tag>
          </div>
          <div class="mt-1 text-xs text-gray-500">
            {{ activeSyncCompletedCount }}/{{ activeSyncBatch.totalCount }}，失败 {{ activeSyncBatch.failedCount }}
          </div>
        </div>
        <div class="flex w-full flex-col gap-3 md:w-80 md:items-end">
          <div class="w-full">
            <el-progress :percentage="activeSyncProgress" :status="activeSyncBatch.status === 'failed' ? 'exception' : undefined" />
          </div>
          <button
            v-if="canRetryActiveSyncBatch"
            @click="handleRetryFailedSyncBatch"
            :disabled="retryingSyncBatch"
            class="btn-ember inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md disabled:opacity-70"
          >
            <el-icon><Refresh /></el-icon>
            <span>{{ retryingSyncBatch ? '重试中...' : '重试失败项' }}</span>
          </button>
        </div>
      </div>

      <div
        v-if="activeSyncFailedUsers.length > 0"
        class="mt-4 rounded-xl border border-red-100 bg-red-50/60 p-3"
      >
        <div class="mb-2 text-xs font-semibold text-red-700">失败用户</div>
        <div class="space-y-1.5">
          <div
            v-for="item in activeSyncFailedUsers"
            :key="item.userId || item.embyId || item.username || item.error"
            class="grid gap-1 text-xs md:grid-cols-[minmax(8rem,12rem)_1fr] md:gap-3"
          >
            <!-- users.username 在 schema 上为 NOT NULL（infrastructure/database/00000000_baseline_20260605.sql），
                 后端 buildEmbyPolicySyncBatchDetail 通过 JOIN users 读取，正常必有值；保留兜底以备异常。 -->
            <span class="min-w-0 truncate font-medium text-gray-800">
              {{ item.username || item.userId || item.embyId || '未知用户' }}
            </span>
            <span class="text-red-600 md:text-right">{{ item.error }}</span>
          </div>
        </div>
        <div v-if="activeSyncHiddenFailedUserCount > 0" class="mt-2 text-xs text-red-500">
          另有 {{ activeSyncHiddenFailedUserCount }} 个失败用户
        </div>
      </div>
    </div>

    <EmberTableCard :data="groups" :loading="loading">
      <template #header>
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-gray-900">分组列表</h2>
          <span class="text-sm text-gray-500">权益模板随分组生效</span>
        </div>
      </template>

      <el-table-column label="分组" min-width="240">
        <template #default="{ row }">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-orange-50 text-orange-500">
              <el-icon><CollectionTag /></el-icon>
            </div>
            <div>
              <div class="flex items-center gap-2">
                <span class="font-semibold text-gray-900">{{ row.name }}</span>
                <el-tag v-if="row.isDefault" type="warning" effect="light" round size="small">默认</el-tag>
              </div>
              <div class="text-xs text-gray-500">{{ row.key }}</div>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="说明" min-width="220">
        <template #default="{ row }">
          <span class="text-sm text-gray-600">{{ row.description || '无说明' }}</span>
        </template>
      </el-table-column>

      <el-table-column label="媒体库" width="110">
        <template #default="{ row }">
          <span class="font-medium text-gray-700">{{ row.mediaLibraryCount ?? 0 }}</span>
        </template>
      </el-table-column>

      <el-table-column label="套餐数" width="100">
        <template #default="{ row }">
          <span class="font-medium text-gray-700">{{ row.planCount ?? 0 }}</span>
        </template>
      </el-table-column>

      <el-table-column label="显式用户" width="110">
        <template #default="{ row }">
          <span class="font-medium text-gray-700">{{ row.userCount ?? 0 }}</span>
        </template>
      </el-table-column>

      <el-table-column label="同步" width="110">
        <template #default="{ row }">
          <el-tag :type="resolveGroupPolicySyncPresentation(row.policySyncStatus).tagType" effect="light" round size="small">
            {{ resolveGroupPolicySyncPresentation(row.policySyncStatus).label }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column label="自动通过" width="120">
        <template #default="{ row }">
          <span class="font-medium text-gray-700">{{ row.subscriptionAutoApproveDailyLimit ?? 0 }}/天</span>
        </template>
      </el-table-column>

      <el-table-column label="排序" width="90">
        <template #default="{ row }">
          <span class="text-gray-600">{{ row.sortOrder }}</span>
        </template>
      </el-table-column>

      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <div class="flex items-center justify-end gap-1">
            <el-tooltip content="媒体库模板" placement="top">
              <button
                @click="openMediaDialog(row)"
                class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-orange-50 hover:text-orange-600"
                aria-label="配置媒体库模板"
              >
                <el-icon :size="18"><FolderOpened /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip content="Emby 权益模板" placement="top">
              <button
                @click="openPolicyDialog(row)"
                class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
                aria-label="配置 Emby 权益模板"
              >
                <el-icon :size="18"><Setting /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip content="编辑" placement="top">
              <button
                @click="openEditDialog(row)"
                class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
                aria-label="编辑用户分组"
              >
                <el-icon :size="18"><EditPen /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip content="删除" placement="top">
              <button
                v-if="canDeletePlanGroup(row)"
                @click="handleDelete(row)"
                class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
                aria-label="删除用户分组"
              >
                <el-icon :size="18"><Delete /></el-icon>
              </button>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
    </EmberTableCard>

    <EmberFormDialog
      v-model="dialogVisible"
      title="新建用户分组"
      width="520px"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="分组标识">
            <el-input v-model="createForm.key" placeholder="例如：VIP_A" class="input-ember" />
            <p class="mt-1 text-xs text-gray-500">分组标识会作为稳定引用保存，创建后不支持修改。</p>
          </el-form-item>

          <el-form-item label="分组名称">
            <el-input v-model="createForm.name" placeholder="例如：新客优惠组" class="input-ember" />
          </el-form-item>

          <el-form-item label="说明">
            <el-input v-model="createForm.description" type="textarea" :rows="3" placeholder="可选，说明这个分组给谁用" class="input-ember" />
          </el-form-item>

          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <el-form-item label="排序">
              <el-input-number v-model="createForm.sortOrder" :min="0" class="w-full !w-full form-number" />
            </el-form-item>

            <el-form-item label="默认分组">
              <div class="flex h-10 items-center justify-between rounded-xl border border-gray-200 bg-gray-50 px-3">
                <el-switch v-model="createForm.isDefault" active-text="是" inactive-text="否" />
              </div>
            </el-form-item>
          </div>

          <el-form-item label="每日自动通过订阅数">
            <el-input-number
              v-model="createForm.subscriptionAutoApproveDailyLimit"
              :min="0"
              :step="1"
              :precision="0"
              class="w-full !w-full form-number"
            />
            <p class="mt-1 text-xs text-gray-500">0 表示该分组全部订阅都进入人工审核。</p>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="dialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 cursor-pointer"
          >
            取消
          </button>
          <button
            @click="handleCreate"
            :disabled="creating"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70 cursor-pointer"
          >
            {{ creating ? '创建中...' : '确认创建' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="editDialogVisible"
      title="编辑用户分组"
      width="520px"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="分组标识">
            <el-input :model-value="editForm.key" disabled class="input-ember" />
          </el-form-item>

          <el-form-item label="分组名称">
            <el-input v-model="editForm.name" placeholder="例如：新客优惠组" class="input-ember" />
          </el-form-item>

          <el-form-item label="说明">
            <el-input v-model="editForm.description" type="textarea" :rows="3" placeholder="可选，说明这个分组给谁用" class="input-ember" />
          </el-form-item>

          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <el-form-item label="排序">
              <el-input-number v-model="editForm.sortOrder" :min="0" class="w-full !w-full form-number" />
            </el-form-item>

            <el-form-item label="默认分组">
              <div class="flex h-10 items-center justify-between rounded-xl border border-gray-200 bg-gray-50 px-3">
                <el-switch v-model="editForm.isDefault" active-text="是" inactive-text="否" />
              </div>
            </el-form-item>
          </div>

          <el-form-item label="每日自动通过订阅数">
            <el-input-number
              v-model="editForm.subscriptionAutoApproveDailyLimit"
              :min="0"
              :step="1"
              :precision="0"
              class="w-full !w-full form-number"
            />
            <p class="mt-1 text-xs text-gray-500">0 表示该分组全部订阅都进入人工审核。</p>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="editDialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 cursor-pointer"
          >
            取消
          </button>
          <button
            @click="handleUpdate"
            :disabled="updating"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70 cursor-pointer"
          >
            {{ updating ? '保存中...' : '保存修改' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="mediaDialogVisible"
      title="媒体库模板"
      width="680px"
    >
      <div class="p-6 pt-2">
        <div v-loading="loadingTemplate" class="space-y-4">
          <div class="flex items-center justify-between rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3">
            <div>
              <div class="text-sm font-semibold text-gray-900">{{ selectedGroup?.name || '-' }}</div>
              <div class="text-xs text-gray-500">{{ selectedGroup?.key || '-' }}</div>
            </div>
            <el-tag type="info" effect="light" round>{{ selectedLibraryIds.length }} 个媒体库</el-tag>
          </div>

          <EmberEmptyStateCard
            v-if="!loadingTemplate && allLibraries.length === 0"
            title="暂无可选媒体库"
            description="请先确认 Emby 配置和媒体库同步状态。"
            :icon="FolderOpened"
            compact
          />

          <div v-else class="grid max-h-[28rem] gap-3 overflow-y-auto pr-1 md:grid-cols-2">
            <label
              v-for="library in allLibraries"
              :key="library.id"
              class="flex cursor-pointer items-start gap-3 rounded-2xl border border-gray-200 bg-white p-4 transition-colors hover:border-ember/40 hover:bg-ember/5"
            >
              <el-checkbox v-model="selectedLibraryIds" :value="library.id" size="large" />
              <span class="min-w-0">
                <span class="block truncate text-sm font-semibold text-gray-900">{{ library.name }}</span>
                <span class="mt-1 block text-xs text-gray-500">{{ formatMediaLibrarySummary(library) }}</span>
              </span>
            </label>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="mediaDialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 cursor-pointer"
          >
            取消
          </button>
          <button
            @click="handleSaveMediaLibraries(false)"
            :disabled="savingLibraries || loadingTemplate"
            class="rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100 disabled:opacity-70 cursor-pointer"
          >
            {{ savingLibraries && savingLibrariesMode === 'deferred' ? '保存中...' : '仅保存模板' }}
          </button>
          <button
            @click="handleSaveMediaLibraries(true)"
            :disabled="savingLibraries || loadingTemplate"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70 cursor-pointer"
          >
            {{ savingLibraries && savingLibrariesMode === 'batch' ? '保存中...' : '保存并同步现有用户' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="policyDialogVisible"
      title="Emby 权益模板"
      width="680px"
    >
      <div class="p-6 pt-2">
        <el-form v-loading="loadingTemplate" label-position="top" class="space-y-4">
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3">
            <div class="text-sm font-semibold text-gray-900">{{ selectedGroup?.name || '-' }}</div>
            <div class="text-xs text-gray-500">{{ selectedGroup?.key || '-' }}</div>
          </div>

          <el-form-item label="同时播放数">
            <el-input-number
              v-model="policyForm.simultaneousStreamLimit"
              :min="0"
              :max="99"
              class="w-full !w-full form-number"
            />
          </el-form-item>

          <div class="grid gap-3 md:grid-cols-2">
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
              <span class="text-sm font-medium text-gray-700">允许下载</span>
              <el-switch v-model="policyForm.enableContentDownloading" />
            </div>
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
              <span class="text-sm font-medium text-gray-700">Live TV</span>
              <el-switch v-model="policyForm.enableLiveTvAccess" />
            </div>
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
              <span class="text-sm font-medium text-gray-700">同步转码</span>
              <el-switch v-model="policyForm.enableSyncTranscoding" />
            </div>
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
              <span class="text-sm font-medium text-gray-700">音频转码</span>
              <el-switch v-model="policyForm.enableAudioPlaybackTranscoding" />
            </div>
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
              <span class="text-sm font-medium text-gray-700">视频转码</span>
              <el-switch v-model="policyForm.enableVideoPlaybackTranscoding" />
            </div>
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
              <span class="text-sm font-medium text-gray-700">Remux</span>
              <el-switch v-model="policyForm.enablePlaybackRemuxing" />
            </div>
            <div class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3 md:col-span-2">
              <span class="text-sm font-medium text-gray-700">远程访问</span>
              <el-switch v-model="policyForm.enableRemoteAccess" />
            </div>
          </div>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="policyDialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 cursor-pointer"
          >
            取消
          </button>
          <button
            @click="handleSavePolicyTemplate"
            :disabled="savingPolicy || loadingTemplate"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70 cursor-pointer"
          >
            {{ savingPolicy ? '保存中...' : '保存模板' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>
