<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Iphone, Plus, RefreshRight, Search, WarningFilled, SwitchButton } from '@element-plus/icons-vue'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import EmberTableCard from '@/components/ember/data-display/EmberTableCard.vue'
import EmberSearchInput from '@/components/ember/filters/EmberSearchInput.vue'
import EmberSelectField from '@/components/ember/filters/EmberSelectField.vue'
import EmberFilterPanel from '@/components/ember/layout/EmberFilterPanel.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import { formatDateTime } from '@/utils/date'
import {
  addDeviceBlacklist,
  getDeviceActions,
  getDeviceBlacklist,
  getDevices,
  getDeviceStats,
  logoutBlacklistedDevices,
  logoutDevice,
  removeDeviceBlacklist
} from '@/api/admin'
import type { ClientBlacklist, DeviceAction, DeviceItem, DeviceStats } from '@/types/api'

const loading = ref(false)
const blacklistsLoading = ref(false)
const statsLoading = ref(false)
const actionsLoading = ref(false)
const submitting = ref(false)
const batchProcessing = ref(false)

const deviceList = ref<DeviceItem[]>([])
const total = ref(0)
const stats = ref<DeviceStats>({
  clientDistribution: [],
  topDevices: [],
  blacklistedClientCount: 0,
  activeSessionCount: 0
})
const blacklists = ref<ClientBlacklist[]>([])
const actions = ref<DeviceAction[]>([])
let fetchDevicesRequestToken = 0

const query = ref({
  page: 1,
  pageSize: 20,
  userId: '',
  clientName: '',
  isBlacklisted: '' as '' | 'true' | 'false'
})

const blacklistForm = ref({
  clientName: '',
  reason: ''
})

const blacklistedInCurrentPage = computed(() => deviceList.value.filter((item) => item.isBlacklisted).length)

const handleDeviceSearch = () => {
  query.value.page = 1
  fetchDevices()
}

const handleDeviceReset = () => {
  query.value = {
    page: 1,
    pageSize: 20,
    userId: '',
    clientName: '',
    isBlacklisted: ''
  }
  fetchDevices()
}

const handleDevicePageSizeChange = (size: number) => {
  query.value.pageSize = size
  query.value.page = 1
  fetchDevices()
}

const fetchDevices = async () => {
  const requestToken = ++fetchDevicesRequestToken
  loading.value = true
  try {
    const params: Record<string, string | number | boolean> = {
      page: query.value.page,
      pageSize: query.value.pageSize
    }
    if (query.value.userId.trim()) {
      params.userId = query.value.userId.trim()
    }
    if (query.value.clientName.trim()) {
      params.clientName = query.value.clientName.trim()
    }
    if (query.value.isBlacklisted === 'true') {
      params.isBlacklisted = true
    }
    if (query.value.isBlacklisted === 'false') {
      params.isBlacklisted = false
    }

    const res = await getDevices(params)
    if (requestToken !== fetchDevicesRequestToken) {
      return
    }
    deviceList.value = res.data || []
    total.value = res.total || 0
  } finally {
    if (requestToken === fetchDevicesRequestToken) {
      loading.value = false
    }
  }
}

const fetchStats = async () => {
  statsLoading.value = true
  try {
    const res = await getDeviceStats()
    stats.value = res.data || stats.value
  } finally {
    statsLoading.value = false
  }
}

const fetchBlacklists = async () => {
  blacklistsLoading.value = true
  try {
    const res = await getDeviceBlacklist()
    blacklists.value = res.data || []
  } finally {
    blacklistsLoading.value = false
  }
}

const fetchActions = async () => {
  actionsLoading.value = true
  try {
    const res = await getDeviceActions({ limit: 20 })
    actions.value = res.data || []
  } finally {
    actionsLoading.value = false
  }
}

const refreshAll = async () => {
  await Promise.all([fetchDevices(), fetchStats(), fetchBlacklists(), fetchActions()])
}

const handleAddBlacklist = async () => {
  if (!blacklistForm.value.clientName.trim()) {
    ElMessage.warning('请输入客户端名称')
    return
  }

  submitting.value = true
  try {
    await addDeviceBlacklist({
      clientName: blacklistForm.value.clientName.trim(),
      reason: blacklistForm.value.reason.trim() || undefined
    })
    ElMessage.success('黑名单添加成功')
    blacklistForm.value.clientName = ''
    blacklistForm.value.reason = ''
    await Promise.all([fetchBlacklists(), fetchDevices(), fetchStats(), fetchActions()])
  } finally {
    submitting.value = false
  }
}

const handleRemoveBlacklist = async (clientName: string) => {
  try {
    await ElMessageBox.confirm(`确认将客户端 "${clientName}" 移出黑名单？`, '操作确认', {
      type: 'warning',
      confirmButtonText: '确认移除',
      cancelButtonText: '取消'
    })
    await removeDeviceBlacklist(clientName)
    ElMessage.success('已移除黑名单')
    await Promise.all([fetchBlacklists(), fetchDevices(), fetchStats(), fetchActions()])
  } catch {
    // canceled
  }
}

const handleLogoutDevice = async (row: DeviceItem) => {
  if (!row.deviceId) return

  try {
    await ElMessageBox.confirm(`确认强制注销设备 "${row.deviceName}"？`, '操作确认', {
      type: 'warning',
      confirmButtonText: '确认注销',
      cancelButtonText: '取消'
    })
    await logoutDevice(row.deviceId)
    ElMessage.success('设备已强制注销')
    await Promise.all([fetchDevices(), fetchStats(), fetchActions()])
  } catch {
    // canceled
  }
}

const handleLogoutBlacklistedDevices = async () => {
  try {
    await ElMessageBox.confirm('确认批量注销所有黑名单客户端设备？', '高风险操作', {
      type: 'warning',
      confirmButtonText: '确认执行',
      cancelButtonText: '取消'
    })
    batchProcessing.value = true
    const res = await logoutBlacklistedDevices()
    const successCount = res.successDeviceIds?.length ?? 0
    const failedCount = res.failedDeviceIds?.length ?? 0
    if (failedCount > 0) {
      ElMessage.warning(`批量注销完成，成功 ${successCount} 台，失败 ${failedCount} 台`)
    } else {
      ElMessage.success(`批量注销完成，本次处理 ${successCount} 台设备`)
    }
    await Promise.all([fetchDevices(), fetchStats(), fetchActions()])
  } catch {
    // canceled or error handled
  } finally {
    batchProcessing.value = false
  }
}

const formatTime = (value?: string) => {
  return formatDateTime(value, 'short')
}

const actionLabelMap: Record<string, string> = {
  blacklist: '加入黑名单',
  unblacklist: '移除黑名单',
  logout: '设备注销'
}

onMounted(refreshAll)
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="设备管理"
      description="黑名单治理、设备下线与行为审计"
    >
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">Total: {{ total }}</span>
      </template>
      <template #actions>
        <button
          type="button"
          @click="refreshAll"
          class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-50 cursor-pointer"
        >
          <el-icon><RefreshRight /></el-icon>
          刷新
        </button>
      </template>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
        <EmberMetricCard
          v-loading="statsLoading"
          title="活跃播放会话"
          :value="stats.activeSessionCount"
          detail="当前在线播放设备数"
        />
        <EmberMetricCard
          v-loading="statsLoading"
          title="黑名单客户端"
          :value="stats.blacklistedClientCount"
          detail="命中后可统一执行下线"
        />
        <EmberMetricCard
          title="当前页命中黑名单"
          :value="blacklistedInCurrentPage"
          detail="随当前分页结果变化"
        />
        <EmberMetricCard
          v-loading="statsLoading"
          title="客户端种类数"
          :value="stats.clientDistribution.length"
          detail="按客户端名称聚合"
        />
      </div>

      <EmberFilterPanel
        wrapper-class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_auto]"
        content-class="grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-3"
        actions-class="flex items-end justify-end gap-2"
      >
        <EmberSearchInput
          v-model="query.userId"
          label="用户 ID"
          aria-label="按用户 ID 筛选"
          placeholder="按用户 ID 筛选"
          :icon="Search"
          @enter="handleDeviceSearch"
        />

        <EmberSearchInput
          v-model="query.clientName"
          label="客户端"
          aria-label="按客户端筛选"
          placeholder="按客户端筛选"
          :icon="Iphone"
          @enter="handleDeviceSearch"
        />

        <EmberSelectField
          v-model="query.isBlacklisted"
          label="黑名单状态"
          placeholder="全部状态"
        >
          <el-option label="全部状态" value="" />
          <el-option label="仅黑名单" value="true" />
          <el-option label="仅非黑名单" value="false" />
        </EmberSelectField>

        <template #actions>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-100 cursor-pointer"
            @click="handleDeviceReset"
          >
            <el-icon><RefreshRight /></el-icon>
            重置
          </button>
          <button
            type="button"
            class="btn-ember inline-flex items-center gap-1.5 rounded-xl px-4 py-2.5 text-sm font-semibold cursor-pointer"
            @click="handleDeviceSearch"
          >
            <el-icon><Search /></el-icon>
            查询
          </button>
        </template>
      </EmberFilterPanel>
    </EmberPageHeaderCard>

    <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
      <div class="bg-white border border-gray-100 rounded-2xl shadow-sm p-6 space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-gray-900">客户端黑名单</h2>
          <button
            type="button"
            @click="handleLogoutBlacklistedDevices"
            :disabled="batchProcessing"
            class="px-3 py-2 rounded-lg bg-red-600 text-white text-sm font-semibold hover:bg-red-700 transition-colors disabled:opacity-60 flex items-center gap-2"
          >
            <el-icon><SwitchButton /></el-icon>
            一键注销黑名单设备
          </button>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
          <div class="space-y-1.5">
            <label class="text-xs font-semibold tracking-wide text-gray-500">客户端名称</label>
            <el-input
              v-model="blacklistForm.clientName"
              placeholder="客户端名称，例如 Infuse"
              aria-label="客户端名称"
              class="input-ember"
            />
          </div>
          <div class="space-y-1.5">
            <label class="text-xs font-semibold tracking-wide text-gray-500">原因</label>
            <el-input
              v-model="blacklistForm.reason"
              placeholder="原因（可选）"
              aria-label="黑名单原因"
              class="input-ember"
            />
          </div>
          <div class="flex items-end">
            <button
              type="button"
              @click="handleAddBlacklist"
              :disabled="submitting"
              class="btn-ember flex w-full items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold"
            >
              <el-icon><Plus /></el-icon>
              添加黑名单
            </button>
          </div>
        </div>

        <el-table :data="blacklists" v-loading="blacklistsLoading" size="small" style="width: 100%">
          <el-table-column prop="clientName" label="客户端" min-width="140" />
          <el-table-column prop="reason" label="原因" min-width="150" />
          <el-table-column label="创建时间" min-width="170">
            <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <button
                type="button"
                class="text-red-600 hover:text-red-700 font-semibold"
                @click="handleRemoveBlacklist(row.clientName)"
              >
                移除
              </button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="bg-white border border-gray-100 rounded-2xl shadow-sm p-6">
        <h2 class="text-lg font-bold text-gray-900 mb-4">设备分布 Top</h2>
        <div class="space-y-3" v-loading="statsLoading">
          <div
            v-for="item in stats.topDevices.slice(0, 8)"
            :key="item.deviceName"
            class="flex items-center justify-between rounded-xl border border-gray-100 px-3 py-2"
          >
            <span class="text-sm text-gray-700 truncate">{{ item.deviceName }}</span>
            <span class="text-xs font-semibold text-gray-500">{{ item.count }}</span>
          </div>
          <div v-if="stats.topDevices.length === 0" class="text-sm text-gray-400">暂无数据</div>
        </div>
      </div>
    </div>

    <EmberTableCard :data="deviceList" :loading="loading">
      <el-table-column prop="deviceName" label="设备" min-width="150" show-overflow-tooltip />
      <el-table-column prop="clientName" label="客户端" min-width="140" show-overflow-tooltip />
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <el-tag :type="row.isActive ? 'success' : 'info'" effect="light">
            {{ row.isActive ? '在线' : '离线' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="黑名单" width="120">
        <template #default="{ row }">
          <el-tag v-if="row.isBlacklisted" type="danger" effect="light">
            命中
          </el-tag>
          <span v-else class="text-gray-400">否</span>
        </template>
      </el-table-column>
      <el-table-column label="用户" min-width="160">
        <template #default="{ row }">
          <span class="text-gray-700">{{ row.userName || '-' }}</span>
          <div class="text-xs text-gray-400">{{ row.userId || '-' }}</div>
        </template>
      </el-table-column>
      <el-table-column label="最后活动" min-width="170">
        <template #default="{ row }">{{ formatTime(row.lastActivityDate) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <button
            type="button"
            class="px-2 py-1 rounded-md bg-red-50 text-red-600 hover:bg-red-100 text-sm font-semibold"
            @click="handleLogoutDevice(row)"
          >
            强制注销
          </button>
        </template>
      </el-table-column>
      <template #pagination>
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @current-change="fetchDevices"
          @size-change="handleDevicePageSizeChange"
        />
      </template>
    </EmberTableCard>

    <div class="bg-white border border-gray-100 rounded-2xl shadow-sm p-6">
      <div class="flex items-center gap-2 mb-4">
        <el-icon class="text-orange-500"><WarningFilled /></el-icon>
        <h2 class="text-lg font-bold text-gray-900">最近操作日志</h2>
      </div>
      <el-table :data="actions" v-loading="actionsLoading" size="small" style="width: 100%">
        <el-table-column label="操作" width="130">
          <template #default="{ row }">{{ actionLabelMap[row.action] || row.action }}</template>
        </el-table-column>
        <el-table-column prop="clientName" label="客户端" min-width="140" />
        <el-table-column prop="deviceId" label="设备ID" min-width="180" show-overflow-tooltip />
        <el-table-column prop="userId" label="用户ID" min-width="160" show-overflow-tooltip />
        <el-table-column prop="note" label="备注" min-width="150" />
        <el-table-column label="时间" min-width="180">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>
