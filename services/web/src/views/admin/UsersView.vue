<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
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
import {
  applyPlanGroupMediaLibrarySync,
  clearAdminUserMediaLibraryPreferences,
  createAdminUser,
  deleteUser,
  extendUserExpiry,
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
const createDialogVisible = ref(false)
const editDialogVisible = ref(false)
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

const planGroupOptions = computed(() => planGroups.value.map(group => ({
  label: `${group.name} (${group.key})`,
  value: group.key
})))

const defaultPlanGroup = computed(() => planGroups.value.find(group => group.isDefault) ?? null)
const usernamePattern = /^[A-Za-z0-9]+$/

// ... (Keep existing logic methods) ...
const isMessageBoxCancel = (error: unknown) => error === 'cancel' || error === 'close'

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

const selectedFilterPlanGroup = computed(() => {
  const key = queryParams.value.planGroup
  return key ? planGroups.value.find(group => group.key === key) ?? null : null
})

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

const handleOpenEdit = (row: UserInfo) => {
  editForm.value = {
    id: row.id,
    email: row.email || '',
    isActive: row.isActive,
    planGroup: row.effectivePlanGroup || row.planGroup || defaultPlanGroup.value?.key || '',
    neverExpire: !row.expiresAt,
    expiresAt: row.expiresAt ? new Date(row.expiresAt) : null
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

  const payload: UpdateAdminUserRequest = {
    email,
    isActive: editForm.value.isActive,
    planGroup: editForm.value.planGroup,
    clearExpiresAt: editForm.value.neverExpire
  }

  if (!editForm.value.neverExpire && editForm.value.expiresAt) {
    payload.expiresAt = editForm.value.expiresAt.toISOString()
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
    await ElMessageBox.prompt('请输入延长天数', '延长到期时间', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputPattern: /^\d+$/,
      inputErrorMessage: '请输入数字',
      inputValue: '30'
    }).then(async ({ value }) => {
      await extendUserExpiry(row.id, parseInt(value, 10))
      ElMessage.success('延长成功')
      await fetchData()
    })
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
    const { value } = await ElMessageBox.prompt('请输入新密码 (留空生成随机密码)', '重置密码', {
      confirmButtonText: '确定',
      cancelButtonText: '取消'
    })

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

const summarizeSyncPreview = (preview: MediaLibrarySyncPreviewResult) => {
  const failedText = preview.failedItems.length > 0 ? `，读取失败 ${preview.failedItems.length} 个` : ''
  return `扫描 ${preview.scannedUsers}/${preview.totalUsers} 个用户，候选模板 ${preview.candidates.length} 个${failedText}`
}

const handleSyncHistoryLibraries = async () => {
  const group = selectedFilterPlanGroup.value
  if (!group) {
    ElMessage.warning('请先在筛选区选择一个套餐组')
    return
  }

  syncingHistoryLibraries.value = true
  try {
    const previewRes = await previewPlanGroupMediaLibrarySync(group.key)
    const preview = previewRes.data
    if (preview.candidates.length === 0) {
      ElMessage.warning('当前分组没有可同步的 Emby 媒体库权限')
      return
    }
    if (preview.candidates.length > 1) {
      await ElMessageBox.alert(
        `${summarizeSyncPreview(preview)}。当前分组内用户媒体库集合不一致，请先处理差异用户后再一键同步。`,
        '历史同步预览',
        { confirmButtonText: '知道了' }
      )
      return
    }

    const candidate = preview.candidates[0]
    await ElMessageBox.confirm(
      `${summarizeSyncPreview(preview)}。确认将 ${group.name} 的媒体库模板同步为 ${candidate.libraryIds.length} 个媒体库吗？`,
      '历史用户媒体库同步',
      {
        confirmButtonText: '确认同步',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    await applyPlanGroupMediaLibrarySync(group.key, { libraryIds: candidate.libraryIds })
    ElMessage.success('已创建历史用户媒体库同步任务')
    await fetchData()
  } catch (error) {
    if (!isMessageBoxCancel(error)) {
      // request interceptor 已处理错误提示
    }
  } finally {
    syncingHistoryLibraries.value = false
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

  if (!row.isActive) {
    return {
      text: '禁用',
      dotClass: 'bg-red-500',
      textClass: 'text-red-700',
      pulse: false,
      reason: '跟随 Ember 禁用'
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
  const status = row.policySyncStatus
  const syncing = status === 'pending' || status === 'processing'
  return {
    customized: row.mediaLibraryPreferenceCustomized === true,
    countText: typeof row.mediaLibraryEnabledCount === 'number' && typeof row.mediaLibraryTemplateCount === 'number'
      ? `${row.mediaLibraryEnabledCount}/${row.mediaLibraryTemplateCount}`
      : '-',
    statusText: syncing ? '同步中' : status === 'failed' ? '同步失败' : status === 'partial_failed' ? '部分失败' : '已同步',
    tagType: syncing ? 'warning' : status === 'failed' || status === 'partial_failed' ? 'danger' : 'success'
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
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">Total: {{ total }}</span>
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
            <span class="font-mono text-gray-600 bg-gray-50 px-2 py-1 rounded text-xs">{{ row.embyId }}</span>
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
                  <el-dropdown-menu class="w-40">
                    <el-dropdown-item :icon="DataLine" @click="handleViewProfile(row)">用户画像</el-dropdown-item>
                    <el-dropdown-item :icon="CreditCard" @click="handleViewPayments(row)">支付记录</el-dropdown-item>
                    <el-dropdown-item :icon="Key" @click="handleResetPassword(row)">重置密码</el-dropdown-item>
                    <el-dropdown-item :icon="RefreshRight" @click="handleSyncMediaLibraryPreferencesFromEmby(row)">同步媒体库偏好</el-dropdown-item>
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
          @size-change="fetchData"
          @current-change="fetchData"
          background
        />
      </template>
    </EmberTableCard>

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
              class="w-full !w-full input-ember"
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
              class="w-full !w-full input-ember"
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

<style scoped>
:deep(.el-table) {
  --el-table-header-bg-color: #f9fafb;
  --el-table-row-hover-bg-color: #fef2f2; /* Light red hover */
}

:deep(.el-table__inner-wrapper::before) {
  display: none; /* Remove bottom border */
}

</style>
