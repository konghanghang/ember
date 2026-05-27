<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
  getPlanGroupEmbyPolicyTemplate,
  getPlanGroupMediaLibraries,
  getPlanGroups,
  updatePlanGroup,
  updatePlanGroupEmbyPolicyTemplate,
  updatePlanGroupMediaLibraries
} from '@/api/admin'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import type {
  CreatePlanGroupRequest,
  EmbyPolicySyncBatchCreated,
  ManagedPlanGroup,
  MediaLibraryOption,
  PlanGroupEmbyPolicyTemplateUpdateRequest,
  UpdatePlanGroupRequest
} from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

const loading = ref(false)
const creating = ref(false)
const updating = ref(false)
const savingLibraries = ref(false)
const savingPolicy = ref(false)
const loadingTemplate = ref(false)
const groups = ref<ManagedPlanGroup[]>([])

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
  sortOrder: 0
})

const editForm = ref({
  key: '',
  name: '',
  description: '',
  isDefault: false,
  sortOrder: 0
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

/** 判断请求是否被后端同步闸门拦截，用于把 409 显式转成同步中提示。 */
const isConflictError = (error: unknown) => {
  return typeof error === 'object'
    && error !== null
    && 'response' in error
    && (error as { response?: { status?: number } }).response?.status === 409
}

/** 把模板保存后返回的同步批次压缩成管理员可执行的结果反馈。 */
const showSyncBatchResult = (batch: EmbyPolicySyncBatchCreated) => {
  if (batch.affectedUserCount > 0) {
    ElMessage.success(`模板已保存，已创建 ${batch.affectedUserCount} 个用户同步任务`)
    return
  }
  ElMessage.success('模板已保存')
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
    sortOrder: 0
  }
}

const openEditDialog = (group: ManagedPlanGroup) => {
  editForm.value = {
    key: group.key,
    name: group.name,
    description: group.description ?? '',
    isDefault: group.isDefault,
    sortOrder: group.sortOrder
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
    sortOrder: createForm.value.sortOrder
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
    sortOrder: editForm.value.sortOrder
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
const handleSaveMediaLibraries = async () => {
  if (!selectedGroup.value) return

  savingLibraries.value = true
  try {
    const res = await updatePlanGroupMediaLibraries(selectedGroup.value.key, selectedLibraryIds.value)
    mediaDialogVisible.value = false
    showSyncBatchResult(res.data)
    await fetchData()
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('该分组有同步任务未完成，稍后再保存')
    }
  } finally {
    savingLibraries.value = false
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
    await fetchData()
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('该分组有同步任务未完成，稍后再保存')
    }
  } finally {
    savingPolicy.value = false
  }
}

onMounted(fetchData)
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
            title="刷新列表"
          >
            <el-icon :size="20"><Refresh /></el-icon>
          </button>
          <button
            @click="dialogVisible = true"
            class="btn-ember inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99]"
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

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <div class="text-sm font-semibold text-gray-500">媒体库模板</div>
        <div class="mt-3 text-3xl font-bold text-gray-900">
          {{ mediaLibraryTemplateCount }}
        </div>
        <div class="mt-1 text-sm text-gray-500">已绑定到分组模板的媒体库数量。</div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <div class="text-sm font-semibold text-gray-500">分组引用总量</div>
        <div class="mt-3 text-3xl font-bold text-gray-900">
          {{ referenceCount }}
        </div>
        <div class="mt-1 text-sm text-gray-500">包含套餐绑定数和显式用户绑定数。</div>
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
                title="媒体库模板"
              >
                <el-icon :size="18"><FolderOpened /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip content="Emby 权益模板" placement="top">
              <button
                @click="openPolicyDialog(row)"
                class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
                aria-label="配置 Emby 权益模板"
                title="Emby 权益模板"
              >
                <el-icon :size="18"><Setting /></el-icon>
              </button>
            </el-tooltip>
            <button
              @click="openEditDialog(row)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
              aria-label="编辑用户分组"
              title="编辑"
            >
              <el-icon :size="18"><EditPen /></el-icon>
            </button>
            <button
              @click="handleDelete(row)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
              aria-label="删除用户分组"
              title="删除"
            >
              <el-icon :size="18"><Delete /></el-icon>
            </button>
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
              <div class="flex h-10 items-center">
                <el-switch v-model="createForm.isDefault" active-text="是" inactive-text="否" />
              </div>
            </el-form-item>
          </div>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="dialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleCreate"
            :disabled="creating"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
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
              <div class="flex h-10 items-center">
                <el-switch v-model="editForm.isDefault" active-text="是" inactive-text="否" />
              </div>
            </el-form-item>
          </div>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="editDialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleUpdate"
            :disabled="updating"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
          >
            {{ updating ? '保存中...' : '保存修改' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="mediaDialogVisible"
      title="媒体库模板"
      width="720px"
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
              <el-checkbox v-model="selectedLibraryIds" :label="library.id" size="large" />
              <span class="min-w-0">
                <span class="block truncate text-sm font-semibold text-gray-900">{{ library.name }}</span>
                <span class="mt-1 block text-xs text-gray-500">{{ library.type || 'Unknown' }} · {{ library.itemCount ?? 0 }} 项</span>
              </span>
            </label>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="mediaDialogVisible = false"
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleSaveMediaLibraries"
            :disabled="savingLibraries || loadingTemplate"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
          >
            {{ savingLibraries ? '保存中...' : '保存模板' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="policyDialogVisible"
      title="Emby 权益模板"
      width="640px"
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
            class="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleSavePolicyTemplate"
            :disabled="savingPolicy || loadingTemplate"
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
          >
            {{ savingPolicy ? '保存中...' : '保存模板' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>
