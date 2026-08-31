<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { MessageBoxInputData } from 'element-plus'
import {
  Calendar,
  Search,
  Edit,
  Timer,
  Key,
  Plus,
  RefreshRight,
  Delete,
  MoreFilled,
  Lock,
  Unlock,
  CreditCard,
  DataLine
} from '@element-plus/icons-vue'
import DefaultAvatar from '@/components/common/DefaultAvatar.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberDateField from '@/components/ember/filters/EmberDateField.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { formatDateTime } from '@/utils/date'
import { isMessageBoxCancel } from '@/utils/api-error'
import { resolveUserPolicySyncPresentation } from '@/utils/policy-sync'
import { formatMediaLibrarySummary } from '@/utils/media-library'
import {
  applyAdminUserCurrentPolicySync,
  applyPlanGroupMediaLibrarySync,
  clearAdminUserMediaLibraryPreferences,
  createAdminUser,
  deleteUser,
  extendUserExpiry,
  getAdminMediaLibraries,
  getPlanGroups,
  getUsers,
  previewPlanGroupMediaLibrarySync,
  resetUserPassword,
  syncAdminUserMediaLibraryPreferences,
  toggleUserStatus,
  updateAdminUser,
  updateAdminUserEmbyAccess
} from '@/api/admin'
import type {
  CreateAdminUserRequest,
  ManagedPlanGroup,
  MediaLibraryOption,
  MediaLibrarySyncCandidate,
  MediaLibrarySyncPreviewResult,
  PlanGroup,
  UpdateAdminUserRequest,
  UserInfo,
  UserListQuery
} from '@/types/api'

const router = useRouter()

const tableData = ref<UserInfo[]>([])
const planGroups = ref<ManagedPlanGroup[]>([])
const total = ref(0)
const loading = ref(false)
const creatingUser = ref(false)
const savingUser = ref(false)
const syncingHistoryLibraries = ref(false)
const applyingHistoryLibraries = ref(false)
const createDialogVisible = ref(false)
const editDialogVisible = ref(false)
const syncPreviewDialogVisible = ref(false)
const syncPreviewGroup = ref<ManagedPlanGroup | null>(null)
const syncPreview = ref<MediaLibrarySyncPreviewResult | null>(null)
const syncAvailableLibraries = ref<MediaLibraryOption[]>([])
const selectedSyncCandidateIndex = ref(0)
const selectedSyncLibraryIds = ref<string[]>([])
const selectedPreferenceUserIds = ref<string[]>([])
const queryParams = ref<UserListQuery>({
  page: 1,
  pageSize: 10,
  search: '',
  expiresAfter: undefined,
  embyStatus: '',
  planGroup: ''
})

const createForm = ref({
  username: '',
  email: '',
  password: '',
  planGroup: '' as PlanGroup | '',
  neverExpire: false,
  expiresAt: null as Date | null
})

const editForm = ref({
  id: '',
  email: '',
  isActive: true,
  planGroup: '' as PlanGroup | '',
  neverExpire: false,
  expiresAt: null as Date | null
})

const editOriginal = ref({
  email: '',
  isActive: true,
  planGroup: '' as PlanGroup | '',
  neverExpire: false,
  expiresAt: null as string | null
})

const planGroupOptions = computed(() => planGroups.value.map(group => ({
  label: `${group.name} (${group.key})`,
  value: group.key
})))

const defaultPlanGroup = computed(() => planGroups.value.find(group => group.isDefault) ?? null)
const usernamePattern = /^[A-Za-z0-9]+$/

// ... (Keep existing logic methods) ...
const generatePassword = (length = 16) => {
  const charset = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*'
  const randomValues = new Uint32Array(length)
  window.crypto.getRandomValues(randomValues)
  return Array.from(randomValues, (value) => charset[value % charset.length]).join('')
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getUsers(queryParams.value)
    tableData.value = res.data
    total.value = res.total
  } finally {
    loading.value = false
  }
}

const fetchPlanGroups = async () => {
  const res = await getPlanGroups()
  planGroups.value = res.data ?? []
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleFilterChange = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleResetFilters = () => {
  queryParams.value.search = ''
  queryParams.value.expiresAfter = undefined
  queryParams.value.embyStatus = ''
  queryParams.value.planGroup = ''
  queryParams.value.page = 1
  fetchData()
}

// 切换每页条数时必须回到第 1 页，否则会按旧页码请求越界空页（对齐 MediaGapsView 的正确写法）。
const handlePageSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const buildLibraryKey = (libraryIds: string[]) => [...libraryIds].sort().join('\x00')

const sameLibrarySet = (left: string[], right: string[]) => buildLibraryKey(left) === buildLibraryKey(right)

const selectedFilterPlanGroup = computed(() => {
  const key = queryParams.value.planGroup
  return key ? planGroups.value.find(group => group.key === key) ?? null : null
})

const syncLibraryOptions = computed(() => {
  const libraries = new Map<string, MediaLibraryOption>()
  for (const library of syncAvailableLibraries.value) {
    libraries.set(library.id, library)
  }
  for (const candidate of syncPreview.value?.candidates ?? []) {
    for (const library of candidate.libraries) {
      libraries.set(library.id, library)
    }
  }
  return Array.from(libraries.values()).sort((a, b) => a.name.localeCompare(b.name))
})

const selectedSyncLibraryKey = computed(() => buildLibraryKey(selectedSyncLibraryIds.value))

const selectedSyncDifferenceUsers = computed(() => {
  const selectedKey = selectedSyncLibraryKey.value
  return (syncPreview.value?.differenceUsers ?? []).filter(user => buildLibraryKey(user.libraryIds) !== selectedKey)
})

const selectedPreferenceUserCount = computed(() => selectedPreferenceUserIds.value.length)

const resetCreateForm = () => {
  createForm.value = {
    username: '',
    email: '',
    password: '',
    planGroup: '',
    neverExpire: false,
    expiresAt: null
  }
}

const openCreateDialog = () => {
  if (planGroups.value.length === 0) {
    ElMessage.warning('当前没有可用套餐组，暂时无法创建用户')
    return
  }
  resetCreateForm()
  createDialogVisible.value = true
}

const handleGenerateCreatePassword = () => {
  createForm.value.password = generatePassword()
}

const validateCreateForm = () => {
  const username = createForm.value.username.trim()
  const email = createForm.value.email.trim()
  const password = createForm.value.password

  if (username.length < 3 || username.length > 50) {
    ElMessage.warning('用户名长度必须为 3-50 位')
    return false
  }
  if (!usernamePattern.test(username)) {
    ElMessage.warning('用户名只能包含字母和数字')
    return false
  }
  if (!email) {
    ElMessage.warning('请输入邮箱')
    return false
  }
  if (password.length < 6) {
    ElMessage.warning('密码长度不能小于 6 位')
    return false
  }
  if (!createForm.value.planGroup) {
    ElMessage.warning('请选择套餐组')
    return false
  }
  if (!createForm.value.neverExpire && !createForm.value.expiresAt) {
    ElMessage.warning('请设置到期时间或选择永不过期')
    return false
  }
  return true
}

const showCreateResult = async (username: string, password: string) => {
  await ElMessageBox.alert(
    h('div', { class: 'space-y-3 text-left' }, [
      h('p', { class: 'text-sm leading-6 text-gray-600' }, '用户已创建成功，请立即复制以下账号信息。'),
      h('div', { class: 'rounded-2xl border border-gray-200 bg-gray-50/80 p-4 space-y-3' }, [
        h('div', { class: 'space-y-1' }, [
          h('div', { class: 'text-xs font-semibold tracking-wide text-gray-500' }, '用户名'),
          h('code', { class: 'block rounded-lg bg-white px-3 py-2 text-sm text-gray-800 ring-1 ring-gray-200' }, username)
        ]),
        h('div', { class: 'space-y-1' }, [
          h('div', { class: 'text-xs font-semibold tracking-wide text-gray-500' }, '初始密码'),
          h('code', { class: 'block rounded-lg bg-white px-3 py-2 text-sm text-gray-800 ring-1 ring-gray-200 break-all' }, password)
        ])
      ])
    ]),
    '创建成功',
    {
      confirmButtonText: '我已复制'
    }
  )
}

const handleCreateUser = async () => {
  if (!validateCreateForm()) {
    return
  }

  const payload: CreateAdminUserRequest = {
    username: createForm.value.username.trim(),
    email: createForm.value.email.trim(),
    password: createForm.value.password,
    planGroup: createForm.value.planGroup
  }

  if (createForm.value.neverExpire) {
    payload.neverExpire = true
  } else if (createForm.value.expiresAt) {
    payload.expiresAt = createForm.value.expiresAt.toISOString()
  }

  const createdPassword = payload.password
  const createdUsername = payload.username

  creatingUser.value = true
  try {
    await createAdminUser(payload)
    createDialogVisible.value = false
    resetCreateForm()
    ElMessage.success('用户创建成功')
    await showCreateResult(createdUsername, createdPassword)
    try {
      await fetchData()
    } catch {
      ElMessage.warning('用户已创建成功，但列表刷新失败，请手动刷新')
    }
  } finally {
    creatingUser.value = false
  }
}

// 把后端到期时间规范化为可比较的 ISO 字符串；非法日期（Invalid Date）调 toISOString 会抛 RangeError，这里按无到期时间处理。
const normalizeExpiresAt = (value?: string | null) => {
  if (!value) return null
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? null : parsed.toISOString()
}

const handleOpenEdit = (row: UserInfo) => {
  const expiresAt = normalizeExpiresAt(row.expiresAt)
  const planGroup = row.effectivePlanGroup || row.planGroup || defaultPlanGroup.value?.key || ''
  editForm.value = {
    id: row.id,
    email: row.email || '',
    isActive: row.isActive,
    planGroup,
    neverExpire: !row.expiresAt,
    expiresAt: expiresAt ? new Date(expiresAt) : null
  }
  editOriginal.value = {
    email: row.email || '',
    isActive: row.isActive,
    planGroup,
    neverExpire: !row.expiresAt,
    expiresAt
  }
  editDialogVisible.value = true
}

const handleUpdateUser = async () => {
  const email = editForm.value.email.trim()
  if (!email) {
    ElMessage.warning('邮箱不能为空')
    return
  }

  if (!editForm.value.neverExpire && !editForm.value.expiresAt) {
    ElMessage.warning('请设置到期时间或选择永不过期')
    return
  }

  const payload: UpdateAdminUserRequest = {}
  const currentExpiresAt = editForm.value.neverExpire
    ? null
    : editForm.value.expiresAt?.toISOString() ?? null

  if (email !== editOriginal.value.email) {
    payload.email = email
  }
  if (editForm.value.isActive !== editOriginal.value.isActive) {
    payload.isActive = editForm.value.isActive
  }
  if (editForm.value.planGroup !== editOriginal.value.planGroup) {
    payload.planGroup = editForm.value.planGroup
  }
  if (editForm.value.neverExpire && editForm.value.neverExpire !== editOriginal.value.neverExpire) {
    payload.clearExpiresAt = true
  }
  if (!editForm.value.neverExpire && currentExpiresAt !== editOriginal.value.expiresAt) {
    payload.expiresAt = currentExpiresAt ?? undefined
  }

  if (Object.keys(payload).length === 0) {
    ElMessage.warning('没有需要保存的修改')
    return
  }

  savingUser.value = true
  try {
    await updateAdminUser(editForm.value.id, payload)
    ElMessage.success('用户信息更新成功')
    editDialogVisible.value = false
    await fetchData()
  } finally {
    savingUser.value = false
  }
}

const handleExtend = async (row: UserInfo) => {
  try {
    const result = await ElMessageBox.prompt('请输入延长天数', '延长到期时间', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputPattern: /^\d+$/,
      inputErrorMessage: '请输入数字',
      inputValue: '30'
    }) as MessageBoxInputData
    // prompt 与 confirm 共用包含 Action 的声明；成功分支运行时固定返回输入对象。
    await extendUserExpiry(row.id, parseInt(result.value, 10))
    ElMessage.success('延长成功')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const handleToggle = async (row: UserInfo) => {
  try {
    await toggleUserStatus(row.id)
    ElMessage.success(row.isActive ? '已禁用' : '已启用')
    await fetchData()
  } catch {
    // handled
  }
}

const handleDelete = async (row: UserInfo) => {
  try {
    await ElMessageBox.confirm('确定删除该用户吗？此操作不可恢复', '警告', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await deleteUser(row.id)
    ElMessage.success('删除成功')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const handleResetPassword = async (row: UserInfo) => {
  try {
    const result = await ElMessageBox.prompt('请输入新密码 (留空生成随机密码)', '重置密码', {
      confirmButtonText: '确定',
      cancelButtonText: '取消'
    }) as MessageBoxInputData
    // prompt 与 confirm 共用包含 Action 的声明；成功分支运行时固定返回输入对象。
    const { value } = result

    let password = value
    if (!password) {
      password = generatePassword()
      await ElMessageBox.alert(`已生成随机密码: ${password}`, '提示')
    }

    await resetUserPassword(row.id, password)
    ElMessage.success('密码重置成功')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const summarizeCandidateLibraries = (candidate: MediaLibrarySyncCandidate) => {
  if (candidate.libraries.length === 0) {
    return '无媒体库'
  }
  return candidate.libraries.map(library => library.name).join('、')
}

const resetPreferenceUsersForTemplate = (libraryIds: string[]) => {
  selectedPreferenceUserIds.value = (syncPreview.value?.differenceUsers ?? [])
    .filter(user => !sameLibrarySet(user.libraryIds, libraryIds))
    .map(user => user.userId)
}

const handleSelectSyncCandidate = (index: number) => {
  const candidate = syncPreview.value?.candidates[index]
  if (!candidate) {
    return
  }
  selectedSyncCandidateIndex.value = index
  selectedSyncLibraryIds.value = [...candidate.libraryIds]
  resetPreferenceUsersForTemplate(candidate.libraryIds)
}

const handleManualSyncLibraryChange = () => {
  selectedSyncCandidateIndex.value = -1
  resetPreferenceUsersForTemplate(selectedSyncLibraryIds.value)
}

const handleSyncHistoryLibraries = async () => {
  const group = selectedFilterPlanGroup.value
  if (!group) {
    ElMessage.warning('请先在筛选区选择一个套餐组')
    return
  }

  syncingHistoryLibraries.value = true
  try {
    const [previewRes, librariesRes] = await Promise.all([
      previewPlanGroupMediaLibrarySync(group.key),
      getAdminMediaLibraries()
    ])
    const preview = previewRes.data
    if (preview.candidates.length === 0) {
      ElMessage.warning('当前分组没有可同步的 Emby 媒体库权限')
      return
    }
    syncPreviewGroup.value = group
    syncPreview.value = preview
    syncAvailableLibraries.value = librariesRes.data ?? []
    syncPreviewDialogVisible.value = true
    handleSelectSyncCandidate(0)
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // request interceptor 已处理错误提示
    }
  } finally {
    syncingHistoryLibraries.value = false
  }
}

const handleApplyHistoryLibraries = async () => {
  const group = syncPreviewGroup.value
  const preview = syncPreview.value
  if (!group || !preview) {
    ElMessage.warning('请先生成历史同步预览')
    return
  }

  applyingHistoryLibraries.value = true
  try {
    const preferenceUserIdSet = new Set(selectedSyncDifferenceUsers.value.map(user => user.userId))
    await applyPlanGroupMediaLibrarySync(group.key, {
      libraryIds: selectedSyncLibraryIds.value,
      preferenceUserIds: selectedPreferenceUserIds.value.filter(userId => preferenceUserIdSet.has(userId))
    })
    ElMessage.success('已创建历史用户媒体库同步任务')
    syncPreviewDialogVisible.value = false
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // request interceptor 已处理错误提示
    }
  } finally {
    applyingHistoryLibraries.value = false
  }
}

const handleClearMediaLibraryPreferences = async (row: UserInfo) => {
  try {
    await ElMessageBox.confirm(`确认清除 ${row.username} 的媒体库偏好吗？`, '清除媒体库偏好', {
      confirmButtonText: '清除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await clearAdminUserMediaLibraryPreferences(row.id)
    ElMessage.success('媒体库偏好已清除')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const handleSyncMediaLibraryPreferencesFromEmby = async (row: UserInfo) => {
  try {
    await ElMessageBox.confirm(`确认从 Emby 当前 Policy 同步 ${row.username} 的媒体库偏好吗？`, '同步媒体库偏好', {
      confirmButtonText: '同步',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await syncAdminUserMediaLibraryPreferences(row.id)
    ElMessage.success('媒体库偏好已从 Emby 同步')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const handleApplyCurrentPolicySync = async (row: UserInfo) => {
  try {
    await ElMessageBox.confirm(`确认同步 ${row.username} 当前有效的 Emby Policy 吗？`, '同步到 Emby', {
      confirmButtonText: '同步',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await applyAdminUserCurrentPolicySync(row.id)
    ElMessage.success('Emby Policy 已提交同步')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const handleViewPolicySyncBatch = (row: UserInfo) => {
  if (!row.policySyncBatchId) return
  router.push({
    name: 'console-plan-groups',
    query: { syncBatchId: row.policySyncBatchId }
  })
}

const handleToggleEmbyAccess = async (row: UserInfo) => {
  const nextDisabled = !row.embyAccessDisabled
  try {
    await ElMessageBox.confirm(
      nextDisabled ? `确认禁用 ${row.username} 的 Emby 访问吗？` : `确认恢复 ${row.username} 的 Emby 访问吗？`,
      nextDisabled ? '禁用 Emby 访问' : '恢复 Emby 访问',
      {
        confirmButtonText: nextDisabled ? '禁用' : '恢复',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    await updateAdminUserEmbyAccess(row.id, nextDisabled)
    ElMessage.success(nextDisabled ? 'Emby 访问已禁用' : 'Emby 访问已恢复')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // handled
    }
  }
}

const handleViewPayments = (row: UserInfo) => {
  router.push({
    name: 'console-billing',
    query: { tab: 'payments', userId: row.id }
  })
}

const handleViewProfile = (row: UserInfo) => {
  router.push({
    name: 'console-user-profile',
    params: { id: row.id },
    query: { range: 'today' }
  })
}

const formatDate = (dateStr?: string | null) => {
  if (!dateStr) return '永不过期'
  return formatDateTime(dateStr, 'short')
}

const getPlanGroupDisplay = (row: UserInfo) => {
  if (row.isPlanGroupMissing) {
    return row.effectivePlanGroup ? `分组失效：${row.effectivePlanGroup}` : '分组失效'
  }
  return row.effectivePlanGroupName || row.effectivePlanGroup || row.planGroupName || row.planGroup || '未设置'
}

const isExpired = (dateStr?: string | null) => {
  if (!dateStr) return false
  const timestamp = new Date(dateStr).getTime()
  if (Number.isNaN(timestamp)) return false
  return timestamp < Date.now()
}

const getEmberStatus = (row: UserInfo) => {
  if (!row.isActive) {
    return {
      text: '禁用',
      dotClass: 'bg-red-500',
      textClass: 'text-red-700',
      pulse: false
    }
  }
  return {
    text: '正常',
    dotClass: 'bg-green-500',
    textClass: 'text-green-700',
    pulse: true
  }
}

const getEmbyStatus = (row: UserInfo) => {
  if (!row.embyId) {
    return {
      text: '未关联',
      dotClass: 'bg-gray-400',
      textClass: 'text-gray-600',
      pulse: false,
      reason: '无 Emby 账号'
    }
  }

  if (row.embyAccessDisabled) {
    return {
      text: '禁用',
      dotClass: 'bg-orange-500',
      textClass: 'text-orange-700',
      pulse: false,
      reason: '管理员禁用'
    }
  }

  if (!row.embyDisabled) {
    return {
      text: '可用',
      dotClass: 'bg-green-500',
      textClass: 'text-green-700',
      pulse: true,
      reason: '未禁用'
    }
  }

  if (row.isExpired || isExpired(row.expiresAt ?? null)) {
    return {
      text: '禁用',
      dotClass: 'bg-yellow-500',
      textClass: 'text-yellow-700',
      pulse: false,
      reason: '过期封禁'
    }
  }

  return {
    text: '禁用',
    dotClass: 'bg-orange-500',
    textClass: 'text-orange-700',
    pulse: false,
    reason: '手动/异常禁用'
  }
}

const getMediaLibraryStatus = (row: UserInfo) => {
  const presentation = resolveUserPolicySyncPresentation(row.policySyncStatus)
  const batchStatus = row.policySyncBatchStatus
  return {
    customized: row.mediaLibraryPreferenceCustomized === true,
    countText: typeof row.mediaLibraryEnabledCount === 'number' && typeof row.mediaLibraryTemplateCount === 'number'
      ? `${row.mediaLibraryEnabledCount}/${row.mediaLibraryTemplateCount}`
      : '-',
    statusText: presentation.label,
    tagType: presentation.tagType,
    batchFailed: batchStatus === 'failed' && !!row.policySyncBatchId
  }
}

onMounted(async () => {
  await fetchPlanGroups()
  await fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="用户管理"
      description="管理系统注册用户、人工开通账号及其权限状态"
    >
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">共 {{ total }} 个用户</span>
      </template>
      <template #actions>
        <div class="flex flex-wrap items-center justify-end gap-3">
          <button
            @click="handleSyncHistoryLibraries"
            :disabled="syncingHistoryLibraries"
            class="inline-flex items-center justify-center gap-2 self-start rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-60 cursor-pointer"
          >
            <el-icon><RefreshRight /></el-icon>
            <span>{{ syncingHistoryLibraries ? '同步中...' : '历史同步' }}</span>
          </button>
          <button
            @click="openCreateDialog"
            class="btn-ember inline-flex items-center justify-center gap-2 self-start rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer"
          >
            <el-icon><Plus /></el-icon>
            <span>新建用户</span>
          </button>
        </div>
      </template>

      <EmberFilterPanel
        wrapper-class="flex flex-col gap-3 xl:flex-row xl:items-end"
        content-class="grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-4 flex-1"
        actions-class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0"
      >
        <EmberSearchInput
          v-model="queryParams.search"
          label="关键词"
          aria-label="搜索用户名或邮箱"
          placeholder="输入用户名或邮箱"
          :icon="Search"
          @enter="handleSearch"
        />

        <EmberDateField
          v-model="queryParams.expiresAfter"
          label="到期晚于"
          type="date"
          value-format="YYYY-MM-DD"
          placeholder="选择日期"
          clearable
          @change="handleFilterChange"
        />

        <EmberSelectField
          v-model="queryParams.embyStatus"
          label="Emby 状态"
          placeholder="全部状态"
          :icon="Lock"
          @change="handleFilterChange"
        >
          <el-option label="全部状态" value="" />
          <el-option label="可用" value="available" />
          <el-option label="禁用" value="disabled" />
          <el-option label="未关联" value="unlinked" />
        </EmberSelectField>

        <EmberSelectField
          v-model="queryParams.planGroup"
          label="套餐组"
          placeholder="全部分组"
          :icon="CreditCard"
          @change="handleFilterChange"
        >
          <el-option label="全部分组" value="" />
          <el-option
            v-for="option in planGroupOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </EmberSelectField>

        <template #actions>
            <button
              @click="handleResetFilters"
              class="px-4 py-2.5 text-sm text-gray-700 bg-white border border-gray-200 hover:bg-gray-100 rounded-xl transition-colors cursor-pointer"
            >
              重置
            </button>
            <button
              @click="handleSearch"
              class="btn-ember px-4 py-2.5 text-sm rounded-xl font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer inline-flex items-center gap-1.5"
            >
              <el-icon><Search /></el-icon>
              查询
            </button>
        </template>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <EmberTableCard :data="tableData" :loading="loading">
        <!-- User Info -->
        <el-table-column label="用户" min-width="200">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <DefaultAvatar :name="row.username" size="md" shape="full" />
              <div class="min-w-0">
                <div class="font-bold text-gray-900 truncate">{{ row.username }}</div>
                <div class="text-xs text-gray-500 truncate font-mono">{{ row.email || 'No Email' }}</div>
              </div>
            </div>
          </template>
        </el-table-column>

        <!-- Emby ID -->
        <el-table-column prop="embyId" label="Emby ID" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.embyId" class="font-mono text-gray-600 bg-gray-50 px-2 py-1 rounded text-xs">{{ row.embyId }}</span>
            <span v-else class="text-xs text-gray-400">-</span>
          </template>
        </el-table-column>

        <!-- Ember Status -->
        <el-table-column label="Ember 状态" width="120">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="relative flex h-2.5 w-2.5">
                <span v-if="getEmberStatus(row).pulse" class="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2.5 w-2.5" :class="getEmberStatus(row).dotClass"></span>
              </span>
              <span class="text-sm font-medium" :class="getEmberStatus(row).textClass">
                {{ getEmberStatus(row).text }}
              </span>
            </div>
          </template>
        </el-table-column>

        <!-- Emby Status -->
        <el-table-column label="Emby 状态" min-width="180">
          <template #default="{ row }">
            <div class="space-y-1">
              <div class="flex items-center gap-2">
                <span class="relative flex h-2.5 w-2.5">
                  <span v-if="getEmbyStatus(row).pulse" class="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                  <span class="relative inline-flex rounded-full h-2.5 w-2.5" :class="getEmbyStatus(row).dotClass"></span>
                </span>
                <span class="text-sm font-medium" :class="getEmbyStatus(row).textClass">
                  {{ getEmbyStatus(row).text }}
                </span>
              </div>
              <span class="inline-flex rounded px-2 py-0.5 text-xs bg-gray-100 text-gray-600">
                {{ getEmbyStatus(row).reason }}
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="套餐组" min-width="160">
          <template #default="{ row }">
            <el-tag
              effect="light"
              round
              size="small"
              :type="row.isPlanGroupMissing ? 'danger' : row.isUsingDefaultPlanGroup ? 'warning' : 'success'"
            >
              {{ getPlanGroupDisplay(row) }}
            </el-tag>
            <div v-if="row.isPlanGroupMissing" class="mt-1 text-[11px] text-red-400">请重新绑定有效分组</div>
          </template>
        </el-table-column>

        <el-table-column label="媒体库偏好" min-width="150">
          <template #default="{ row }">
            <div class="space-y-1">
              <div class="text-sm font-medium text-gray-700">{{ getMediaLibraryStatus(row).countText }}</div>
              <div class="flex flex-wrap gap-1">
                <el-tag size="small" effect="light" round :type="getMediaLibraryStatus(row).customized ? 'warning' : 'info'">
                  {{ getMediaLibraryStatus(row).customized ? '自定义' : '跟随分组' }}
                </el-tag>
                <el-tag size="small" effect="light" round :type="getMediaLibraryStatus(row).tagType">
                  {{ getMediaLibraryStatus(row).statusText }}
                </el-tag>
                <el-tag v-if="getMediaLibraryStatus(row).batchFailed" size="small" effect="light" round type="danger">
                  批次失败
                </el-tag>
              </div>
            </div>
          </template>
        </el-table-column>

        <!-- Expiry -->
        <el-table-column label="到期时间" min-width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-2 text-sm text-gray-600">
              <el-icon class="text-gray-400"><Calendar /></el-icon>
              <span>{{ formatDate(row.expiresAt) }}</span>
            </div>
          </template>
        </el-table-column>

        <!-- Operations -->
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <div class="flex items-center justify-end gap-2">
              <el-tooltip content="编辑信息" placement="top">
                <button 
                  @click="handleOpenEdit(row)"
                  aria-label="编辑信息"
                  class="p-2 text-gray-400 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors cursor-pointer"
                >
                  <el-icon :size="18"><Edit /></el-icon>
                </button>
              </el-tooltip>
              
              <el-tooltip content="延长有效期" placement="top">
                <button 
                  @click="handleExtend(row)"
                  aria-label="延长有效期"
                  class="p-2 text-gray-400 hover:text-green-600 hover:bg-green-50 rounded-lg transition-colors cursor-pointer"
                >
                  <el-icon :size="18"><Timer /></el-icon>
                </button>
              </el-tooltip>

              <el-dropdown trigger="click">
                <button
                  aria-label="更多操作"
                  class="p-2 text-gray-400 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors cursor-pointer"
                >
                  <el-icon :size="18"><MoreFilled /></el-icon>
                </button>
                <template #dropdown>
                  <el-dropdown-menu class="w-52">
                    <el-dropdown-item :icon="DataLine" @click="handleViewProfile(row)">用户画像</el-dropdown-item>
                    <el-dropdown-item :icon="CreditCard" @click="handleViewPayments(row)">支付记录</el-dropdown-item>
                    <el-dropdown-item :icon="Key" @click="handleResetPassword(row)">重置密码</el-dropdown-item>
                    <el-dropdown-item
                      v-if="row.role === 'user' && row.embyId"
                      :icon="RefreshRight"
                      @click="handleApplyCurrentPolicySync(row)"
                    >
                      同步到 Emby
                    </el-dropdown-item>
                    <el-dropdown-item
                      v-if="row.policySyncBatchStatus === 'failed' && row.policySyncBatchId"
                      :icon="RefreshRight"
                      @click="handleViewPolicySyncBatch(row)"
                    >
                      查看同步批次
                    </el-dropdown-item>
                    <el-dropdown-item :icon="RefreshRight" @click="handleSyncMediaLibraryPreferencesFromEmby(row)">从 Emby 读取当前偏好</el-dropdown-item>
                    <el-dropdown-item :icon="RefreshRight" @click="handleClearMediaLibraryPreferences(row)">清除媒体库偏好</el-dropdown-item>
                    <el-dropdown-item
                      :icon="row.embyAccessDisabled ? Unlock : Lock"
                      @click="handleToggleEmbyAccess(row)"
                    >
                      {{ row.embyAccessDisabled ? '恢复 Emby 访问' : '禁用 Emby 访问' }}
                    </el-dropdown-item>
                    <el-dropdown-item
                      :icon="row.isActive ? Lock : Unlock"
                      @click="handleToggle(row)"
                    >
                      {{ row.isActive ? '禁用账号' : '启用账号' }}
                    </el-dropdown-item>
                    <el-dropdown-item :icon="Delete" class="text-red-500" divided @click="handleDelete(row)">删除用户</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>

      <template #pagination>
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handlePageSizeChange"
          @current-change="fetchData"
          background
        />
      </template>
    </EmberTableCard>

    <EmberFormDialog
      v-model="syncPreviewDialogVisible"
      title="历史用户媒体库同步"
      width="920px"
      class="rounded-2xl"
    >
      <div v-if="syncPreview && syncPreviewGroup" class="space-y-5 p-6 pt-2">
        <div class="grid gap-3 md:grid-cols-4">
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3">
            <div class="text-xs font-medium text-gray-500">套餐组</div>
            <div class="mt-1 truncate text-sm font-semibold text-gray-900">{{ syncPreviewGroup.name }}</div>
          </div>
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3">
            <div class="text-xs font-medium text-gray-500">扫描用户</div>
            <div class="mt-1 text-sm font-semibold text-gray-900">{{ syncPreview.scannedUsers }}/{{ syncPreview.totalUsers }}</div>
          </div>
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3">
            <div class="text-xs font-medium text-gray-500">候选模板</div>
            <div class="mt-1 text-sm font-semibold text-gray-900">{{ syncPreview.candidates.length }}</div>
          </div>
          <div class="rounded-2xl border border-gray-100 bg-gray-50 px-4 py-3">
            <div class="text-xs font-medium text-gray-500">偏好用户</div>
            <div class="mt-1 text-sm font-semibold text-gray-900">{{ selectedPreferenceUserCount }}/{{ selectedSyncDifferenceUsers.length }}</div>
          </div>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-gray-900">候选集合</h3>
            <el-tag :type="syncPreview.consistent ? 'success' : 'warning'" effect="light" round>
              {{ syncPreview.consistent ? '一致' : '不一致' }}
            </el-tag>
          </div>
          <div class="grid gap-3 md:grid-cols-2">
            <button
              v-for="(candidate, index) in syncPreview.candidates"
              :key="`${index}-${candidate.libraryIds.join(',')}`"
              type="button"
              class="cursor-pointer rounded-2xl border bg-white p-4 text-left transition-colors hover:border-ember/40 hover:bg-ember/5"
              :class="selectedSyncCandidateIndex === index ? 'border-ember ring-4 ring-ember/10' : 'border-gray-200'"
              @click="handleSelectSyncCandidate(index)"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-semibold text-gray-900">候选 {{ index + 1 }}</span>
                <span class="shrink-0 rounded-full bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600">
                  {{ candidate.userCount }} 人 · {{ candidate.libraryIds.length }} 库
                </span>
              </div>
              <div class="mt-2 line-clamp-2 text-xs leading-5 text-gray-500">
                {{ summarizeCandidateLibraries(candidate) }}
              </div>
            </button>
          </div>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-gray-900">模板媒体库</h3>
            <span class="text-xs font-medium text-gray-500">{{ selectedSyncLibraryIds.length }} 个</span>
          </div>
          <div class="grid max-h-64 gap-3 overflow-y-auto pr-1 md:grid-cols-2">
            <label
              v-for="library in syncLibraryOptions"
              :key="library.id"
              class="flex cursor-pointer items-start gap-3 rounded-2xl border border-gray-200 bg-white p-4 transition-colors hover:border-ember/40 hover:bg-ember/5"
            >
              <el-checkbox
                v-model="selectedSyncLibraryIds"
                :value="library.id"
                size="large"
                @change="handleManualSyncLibraryChange"
              />
              <span class="min-w-0">
                <span class="block truncate text-sm font-semibold text-gray-900">{{ library.name }}</span>
                <span class="mt-1 block text-xs text-gray-500">{{ formatMediaLibrarySummary(library) }}</span>
              </span>
            </label>
          </div>
        </div>

        <div v-if="selectedSyncDifferenceUsers.length > 0" class="space-y-3">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-gray-900">写入个人偏好</h3>
            <span class="text-xs font-medium text-gray-500">{{ selectedPreferenceUserCount }} 个用户</span>
          </div>
          <div class="max-h-64 space-y-2 overflow-y-auto pr-1">
            <label
              v-for="user in selectedSyncDifferenceUsers"
              :key="user.userId"
              class="flex cursor-pointer items-start gap-3 rounded-2xl border border-gray-200 bg-white p-4 transition-colors hover:border-ember/40 hover:bg-ember/5"
            >
              <el-checkbox v-model="selectedPreferenceUserIds" :value="user.userId" size="large" />
              <span class="min-w-0 flex-1">
                <span class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-semibold text-gray-900">{{ user.username }}</span>
                  <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-500">{{ user.libraryIds.length }} 库</span>
                </span>
                <span class="mt-1 block truncate text-xs text-gray-500">
                  {{ user.libraries.length > 0 ? user.libraries.map(library => library.name).join('、') : '无媒体库' }}
                </span>
              </span>
            </label>
          </div>
        </div>

        <div v-if="syncPreview.failedItems.length > 0" class="rounded-2xl border border-red-100 bg-red-50 p-4">
          <div class="text-sm font-semibold text-red-700">读取失败 {{ syncPreview.failedItems.length }} 个</div>
          <div class="mt-2 max-h-28 space-y-1 overflow-y-auto text-xs text-red-600">
            <div v-for="item in syncPreview.failedItems" :key="`${item.userId || item.embyId}-${item.error}`">
              {{ item.username || item.userId || item.embyId || '-' }}：{{ item.error }}
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="syncPreviewDialogVisible = false"
            class="cursor-pointer rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleApplyHistoryLibraries"
            :disabled="applyingHistoryLibraries"
            class="btn-ember cursor-pointer rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70"
          >
            {{ applyingHistoryLibraries ? '同步中...' : '确认同步' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="createDialogVisible"
      title="新建用户"
      width="560px"
      class="rounded-2xl"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="用户名">
            <el-input
              v-model="createForm.username"
              placeholder="3-50 位字母或数字"
              class="input-ember"
              autocomplete="off"
            />
          </el-form-item>

          <el-form-item label="电子邮箱">
            <el-input
              v-model="createForm.email"
              placeholder="user@example.com"
              class="input-ember"
              autocomplete="off"
            />
          </el-form-item>

          <el-form-item label="初始密码">
            <div class="w-full space-y-2">
              <div class="flex w-full items-center gap-3">
                <el-input
                  v-model="createForm.password"
                  placeholder="至少 6 位"
                  class="input-ember min-w-0 flex-1"
                  show-password
                  autocomplete="new-password"
                />
                <button
                  type="button"
                  @click="handleGenerateCreatePassword"
                  class="inline-flex h-[42px] shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-xl border border-gray-200 bg-white px-4 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-100 cursor-pointer"
                >
                  <el-icon><RefreshRight /></el-icon>
                  <span>随机生成</span>
                </button>
              </div>
              <p class="text-xs text-gray-500">创建成功后会展示一次账号和初始密码，请管理员及时复制保存。</p>
            </div>
          </el-form-item>

          <el-form-item label="套餐组">
            <el-select v-model="createForm.planGroup" class="w-full form-select" placeholder="选择套餐组">
              <el-option
                v-for="option in planGroupOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="有效期设置">
            <div class="w-full space-y-2">
              <div class="flex w-full items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
                <span class="block text-sm font-medium text-gray-700">永不过期</span>
                <el-switch v-model="createForm.neverExpire" />
              </div>
              <p class="text-xs text-gray-500">关闭后需要手动填写到期时间。</p>
            </div>
          </el-form-item>

          <el-form-item v-if="!createForm.neverExpire" label="到期时间">
            <el-date-picker
              v-model="createForm.expiresAt"
              type="datetime"
              placeholder="选择日期时间"
              :prefix-icon="Calendar"
              class="w-full !w-full input-ember form-date"
            />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="px-6 pb-6 pt-0 flex justify-end gap-3">
          <button
            @click="createDialogVisible = false"
            class="px-4 py-2.5 text-sm text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-xl transition-colors font-medium cursor-pointer"
          >
            取消
          </button>
          <button
            @click="handleCreateUser"
            :disabled="creatingUser"
            class="btn-ember px-6 py-2.5 text-sm rounded-xl font-semibold shadow-sm hover:shadow-md disabled:opacity-70 cursor-pointer"
          >
            {{ creatingUser ? '创建中...' : '确认创建' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <!-- Edit Dialog -->
    <EmberFormDialog
      v-model="editDialogVisible" 
      title="编辑用户" 
      width="520px"
      class="rounded-2xl"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="电子邮箱">
            <el-input 
              v-model="editForm.email" 
              placeholder="user@example.com" 
              class="input-ember" 
            />
          </el-form-item>

          <el-form-item label="账号状态">
            <div class="w-full space-y-2">
              <div class="flex w-full items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
                <span class="block text-sm font-medium text-gray-700">{{ editForm.isActive ? '正常启用' : '已禁用' }}</span>
                <el-switch v-model="editForm.isActive" />
              </div>
              <p class="text-xs text-gray-500">关闭后该用户将无法继续登录和使用服务。</p>
            </div>
          </el-form-item>

          <el-form-item label="套餐组">
            <el-select v-model="editForm.planGroup" class="w-full form-select" placeholder="选择套餐组">
              <el-option
                v-for="option in planGroupOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="有效期设置">
            <div class="w-full space-y-2">
              <div class="flex w-full items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3">
                <span class="block text-sm font-medium text-gray-700">永不过期</span>
                <el-switch v-model="editForm.neverExpire" />
              </div>
              <p class="text-xs text-gray-500">关闭后需要手动填写到期时间。</p>
            </div>
          </el-form-item>

          <el-form-item label="到期时间" v-if="!editForm.neverExpire">
            <el-date-picker
              v-model="editForm.expiresAt"
              type="datetime"
              placeholder="选择日期时间"
              :prefix-icon="Calendar"
              class="w-full !w-full input-ember form-date"
            />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="px-6 pb-6 pt-0 flex justify-end gap-3">
          <button 
            @click="editDialogVisible = false"
            class="px-4 py-2.5 text-sm text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-xl transition-colors font-medium cursor-pointer"
          >
            取消
          </button>
          <button 
            @click="handleUpdateUser" 
            :disabled="savingUser"
            class="btn-ember px-6 py-2.5 text-sm rounded-xl font-semibold shadow-sm hover:shadow-md disabled:opacity-70 cursor-pointer"
          >
            {{ savingUser ? '保存中...' : '保存更改' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>
