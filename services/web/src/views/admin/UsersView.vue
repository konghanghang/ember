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
import { createAdminUser, getPlanGroups, getUsers, updateAdminUser, extendUserExpiry, toggleUserStatus, deleteUser, resetUserPassword } from '@/api/admin'
import type { CreateAdminUserRequest, ManagedPlanGroup, PlanGroup, UpdateAdminUserRequest, UserInfo, UserListQuery } from '@/types/api'

const router = useRouter()

const tableData = ref<UserInfo[]>([])
const planGroups = ref<ManagedPlanGroup[]>([])
const total = ref(0)
const loading = ref(false)
const creatingUser = ref(false)
const savingUser = ref(false)
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
  return new Date(dateStr).toLocaleString()
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

  if (isExpired(row.expiresAt ?? null)) {
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

onMounted(async () => {
  await fetchPlanGroups()
  await fetchData()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header Area -->
    <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
            用户管理
            <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">Total: {{ total }}</span>
          </h1>
          <p class="text-gray-500 text-sm mt-1">管理系统注册用户、人工开通账号及其权限状态</p>
        </div>

        <button
          @click="openCreateDialog"
          class="btn-ember inline-flex items-center justify-center gap-2 self-start rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer"
        >
          <el-icon><Plus /></el-icon>
          <span>新建用户</span>
        </button>
      </div>

      <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
        <div class="flex flex-col xl:flex-row xl:items-end gap-3">
          <div class="grid grid-cols-1 md:grid-cols-2 2xl:grid-cols-4 gap-3 flex-1">
            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">关键词</label>
              <div class="relative w-full group">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Search /></el-icon>
                </div>
                <input
                  v-model="queryParams.search"
                  type="search"
                  inputmode="search"
                  autocomplete="off"
                  aria-label="搜索用户名或邮箱"
                  placeholder="输入用户名或邮箱"
                  class="filter-input w-full pl-10 pr-4"
                  @keyup.enter="handleSearch"
                />
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">到期晚于</label>
              <div class="relative w-full group">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none z-10">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Calendar /></el-icon>
                </div>
                <el-date-picker
                  v-model="queryParams.expiresAfter"
                  type="date"
                  value-format="YYYY-MM-DD"
                  placeholder="选择日期"
                  clearable
                  class="w-full filter-date"
                  @change="handleFilterChange"
                />
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">Emby 状态</label>
              <div class="relative w-full group">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none z-10">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Lock /></el-icon>
                </div>
                <el-select
                  v-model="queryParams.embyStatus"
                  class="w-full filter-select"
                  placeholder="全部状态"
                  @change="handleFilterChange"
                >
                  <el-option label="全部状态" value="" />
                  <el-option label="可用" value="available" />
                  <el-option label="禁用" value="disabled" />
                  <el-option label="未关联" value="unlinked" />
                </el-select>
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-semibold tracking-wide text-gray-500">套餐组</label>
              <div class="relative w-full group">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none z-10">
                  <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><CreditCard /></el-icon>
                </div>
                <el-select
                  v-model="queryParams.planGroup"
                  class="w-full filter-select"
                  placeholder="全部分组"
                  @change="handleFilterChange"
                >
                  <el-option label="全部分组" value="" />
                  <el-option
                    v-for="option in planGroupOptions"
                    :key="option.value"
                    :label="option.label"
                    :value="option.value"
                  />
                </el-select>
              </div>
            </div>
          </div>

          <div class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0">
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
          </div>
        </div>
      </div>
    </div>

    <!-- Users Table -->
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <el-table 
        :data="tableData" 
        v-loading="loading" 
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
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
      </el-table>

      <!-- Pagination -->
      <div class="flex justify-end p-6 border-t border-gray-100 bg-gray-50/50">
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
      </div>
    </div>

    <el-dialog
      v-model="createDialogVisible"
      title="新建用户"
      width="560px"
      align-center
      append-to-body
      class="rounded-2xl"
    >
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
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
          </div>

          <el-form-item label="初始密码">
            <div class="flex flex-col gap-2 sm:flex-row">
              <el-input
                v-model="createForm.password"
                placeholder="至少 6 位"
                class="input-ember sm:flex-1"
                show-password
                autocomplete="new-password"
              />
              <button
                type="button"
                @click="handleGenerateCreatePassword"
                class="inline-flex items-center justify-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-100 cursor-pointer"
              >
                <el-icon><RefreshRight /></el-icon>
                <span>随机生成</span>
              </button>
            </div>
            <p class="mt-1 text-xs text-gray-500">创建成功后会展示一次账号和初始密码，请管理员及时复制保存。</p>
          </el-form-item>

          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
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
              <div class="flex items-center justify-between p-3 border border-gray-200 rounded-xl bg-gray-50 w-full">
                <span class="text-sm text-gray-700">永不过期</span>
                <el-switch v-model="createForm.neverExpire" />
              </div>
            </el-form-item>
          </div>

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
    </el-dialog>

    <!-- Edit Dialog -->
    <el-dialog 
      v-model="editDialogVisible" 
      title="编辑用户" 
      width="520px"
      align-center
      append-to-body
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
          
          <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <el-form-item label="账号状态">
              <div class="flex items-center justify-between p-3 border border-gray-200 rounded-xl bg-gray-50 w-full">
                <span class="text-sm text-gray-700">{{ editForm.isActive ? '正常启用' : '已禁用' }}</span>
                <el-switch v-model="editForm.isActive" />
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
              <div class="flex items-center justify-between p-3 border border-gray-200 rounded-xl bg-gray-50 w-full">
                <span class="text-sm text-gray-700">永不过期</span>
                <el-switch v-model="editForm.neverExpire" />
              </div>
            </el-form-item>
          </div>

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
    </el-dialog>
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

:deep(.el-dialog__header) {
  margin-right: 0;
  border-bottom: 1px solid #f3f4f6;
  padding: 20px 24px;
}

:deep(.el-dialog__body) {
  padding: 0;
}

:deep(.el-dialog__footer) {
  padding: 0;
}

.filter-input {
  background-color: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 0.75rem;
  height: 42px;
  line-height: 1.2;
  font-size: 0.875rem;
  color: #111827;
  outline: none;
  transition: all 0.2s ease;
}

.filter-input::placeholder {
  color: #9ca3af;
}

.filter-input:hover {
  background-color: #ffffff;
}

.filter-input:focus {
  background-color: #ffffff;
  border-color: var(--ember-red);
  box-shadow: 0 0 0 4px rgba(229, 9, 20, 0.1);
}

:deep(.filter-date .el-input__wrapper) {
  height: 42px;
  min-height: 42px;
  background-color: #f9fafb !important;
  border-radius: 0.75rem;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.filter-date:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.filter-date .el-input__wrapper.is-focus) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.filter-date .el-input__inner) {
  height: 100%;
  padding-left: 2.5rem;
  font-size: 0.875rem;
}

:deep(.filter-select .el-select__wrapper) {
  height: 42px;
  min-height: 42px;
  background-color: #f9fafb !important;
  border-radius: 0.75rem;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.filter-select:hover .el-select__wrapper) {
  background-color: #ffffff !important;
}

:deep(.filter-select .el-select__wrapper.is-focused),
:deep(.filter-select .el-select__wrapper.is-focus),
:deep(.filter-select.is-focus .el-select__wrapper) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.filter-select .el-select__selected-item),
:deep(.filter-select .el-select__placeholder) {
  padding-left: 1.8rem;
  font-size: 0.875rem;
}

:deep(.form-select .el-select__wrapper) {
  min-height: 42px;
  border-radius: 0.75rem;
  background-color: #f9fafb !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.form-select:hover .el-select__wrapper) {
  background-color: #ffffff !important;
}

:deep(.form-select .el-select__wrapper.is-focused),
:deep(.form-select .el-select__wrapper.is-focus),
:deep(.form-select.is-focus .el-select__wrapper) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.form-select .el-select__placeholder),
:deep(.form-select .el-select__selected-item) {
  font-size: 0.875rem;
}
</style>
