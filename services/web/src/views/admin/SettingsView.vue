<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Monitor, RefreshRight, Setting } from '@element-plus/icons-vue'
import {
  getConfigs,
  importConfigEnv,
  runCronJob,
  testConfigGroup,
  updateConfig
} from '@/api/admin'
import type { AdminConfigItem } from '@/types/api'

type ConfigGroupKey =
  | 'business'
  | 'media'
  | 'email'
  | 'payment'
  | 'notification'
  | 'schedule'
  | 'deployment'

interface ConfigGroupSection {
  key: ConfigGroupKey
  label: string
  items: AdminConfigItem[]
}

const groupOrder: ConfigGroupKey[] = [
  'business',
  'media',
  'email',
  'payment',
  'notification',
  'schedule',
  'deployment'
]

const loading = ref(false)
const importingEnv = ref(false)
const runningCron = ref(false)
const activeGroup = ref<ConfigGroupKey>('business')
const configs = ref<AdminConfigItem[]>([])
const draftValues = ref<Record<string, any>>({})
const savingGroups = ref<Record<string, boolean>>({})
const testingGroups = ref<Record<string, boolean>>({})

const groupSections = computed<ConfigGroupSection[]>(() => {
  const grouped = new Map<string, ConfigGroupSection>()

  for (const groupKey of groupOrder) {
    const items = configs.value.filter(item => item.group === groupKey)
    if (items.length === 0) continue
    grouped.set(groupKey, {
      key: groupKey,
      label: items[0].groupLabel,
      items
    })
  }

  return groupOrder
    .map(key => grouped.get(key))
    .filter((group): group is ConfigGroupSection => Boolean(group))
})

const configuredCount = computed(() => configs.value.filter(item => item.hasValue).length)
const missingCount = computed(() => configs.value.filter(item => !item.hasValue && item.editable).length)
const sensitiveCount = computed(() => configs.value.filter(item => item.sensitive && item.hasValue).length)
const restartCount = computed(() => configs.value.filter(item => item.restartRequired).length)

const resetDraftValues = (items: AdminConfigItem[]) => {
  const next: Record<string, any> = {}

  for (const item of items) {
    if (item.sensitive) {
      next[item.key] = ''
      continue
    }

    switch (item.type) {
      case 'boolean':
        next[item.key] = item.value === 'true'
        break
      case 'integer':
        next[item.key] = Number(item.value ?? 0)
        break
      case 'json_list':
        if (!item.value) {
          next[item.key] = []
          break
        }
        try {
          const parsed = JSON.parse(item.value)
          next[item.key] = Array.isArray(parsed) ? parsed : []
        } catch {
          next[item.key] = []
        }
        break
      default:
        next[item.key] = item.value ?? ''
    }
  }

  draftValues.value = next
}

const fetchConfigs = async () => {
  const res = await getConfigs()
  configs.value = res.data
  resetDraftValues(res.data)

  if (!groupSections.value.some(group => group.key === activeGroup.value) && groupSections.value.length > 0) {
    activeGroup.value = groupSections.value[0].key
  }
}

const sourceLabelMap: Record<string, string> = {
  database: '数据库',
  env: '环境变量',
  default: '默认值',
  unset: '未设置'
}

const sourceClass = (source: string) => {
  switch (source) {
    case 'database':
      return 'bg-green-50 text-green-700'
    case 'env':
      return 'bg-amber-50 text-amber-700'
    case 'default':
      return 'bg-blue-50 text-blue-700'
    default:
      return 'bg-gray-100 text-gray-600'
  }
}

const restartClass = (restartRequired: boolean) =>
  restartRequired ? 'bg-red-50 text-red-700' : 'bg-emerald-50 text-emerald-700'

const canTestGroup = (group: ConfigGroupKey) => group === 'media' || group === 'email'

const normalizeComparableValue = (item: AdminConfigItem, value: any) => {
  switch (item.type) {
    case 'boolean':
      return value === true
    case 'integer':
      return Number(value ?? 0)
    case 'json_list':
      return JSON.stringify([...(Array.isArray(value) ? value : [])].sort())
    default:
      return String(value ?? '')
  }
}

const currentComparableValue = (item: AdminConfigItem) => {
  if (item.sensitive) {
    return ''
  }

  switch (item.type) {
    case 'boolean':
      return item.value === 'true'
    case 'integer':
      return Number(item.value ?? 0)
    case 'json_list':
      if (!item.value) {
        return JSON.stringify([])
      }
      try {
        const parsed = JSON.parse(item.value)
        return JSON.stringify([...(Array.isArray(parsed) ? parsed : [])].sort())
      } catch {
        return JSON.stringify([])
      }
    default:
      return item.value ?? ''
  }
}

const isItemDirty = (item: AdminConfigItem) => {
  const draftValue = draftValues.value[item.key]
  if (item.sensitive) {
    return String(draftValue ?? '').trim() !== ''
  }
  return normalizeComparableValue(item, draftValue) !== currentComparableValue(item)
}

const groupHasChanges = (group: ConfigGroupSection) =>
  group.items.some(item => item.editable && isItemDirty(item))

const buildUpdatePayload = (item: AdminConfigItem) => {
  const draftValue = draftValues.value[item.key]

  if (item.sensitive) {
    const raw = String(draftValue ?? '')
    if (raw.trim() === '') {
      return null
    }
    return { value: raw }
  }

  switch (item.type) {
    case 'boolean':
      return { value: String(Boolean(draftValue)) }
    case 'integer':
      return { value: String(Number(draftValue ?? 0)) }
    case 'json_list': {
      const values = Array.isArray(draftValue) ? draftValue : []
      return { value: values.length === 0 ? '' : JSON.stringify(values) }
    }
    default:
      return { value: String(draftValue ?? '') }
  }
}

const handleSaveGroup = async (group: ConfigGroupSection) => {
  const changedItems = group.items.filter(item => item.editable && isItemDirty(item))
  if (changedItems.length === 0) {
    ElMessage.info('当前分组没有未保存修改')
    return
  }

  savingGroups.value[group.key] = true
  try {
    for (const item of changedItems) {
      const payload = buildUpdatePayload(item)
      if (!payload) continue
      await updateConfig(item.key, payload)
    }

    ElMessage.success(`${group.label}保存成功`)
    await fetchConfigs()
  } finally {
    savingGroups.value[group.key] = false
  }
}

const handleResetGroup = (group: ConfigGroupSection) => {
  for (const item of group.items) {
    if (item.sensitive) {
      draftValues.value[item.key] = ''
      continue
    }
    switch (item.type) {
      case 'boolean':
        draftValues.value[item.key] = item.value === 'true'
        break
      case 'integer':
        draftValues.value[item.key] = Number(item.value ?? 0)
        break
      case 'json_list':
        if (!item.value) {
          draftValues.value[item.key] = []
          break
        }
        try {
          const parsed = JSON.parse(item.value)
          draftValues.value[item.key] = Array.isArray(parsed) ? parsed : []
        } catch {
          draftValues.value[item.key] = []
        }
        break
      default:
        draftValues.value[item.key] = item.value ?? ''
    }
  }

  ElMessage.success(`${group.label}已恢复到当前生效值`)
}

const handleTestGroup = async (group: ConfigGroupSection) => {
  testingGroups.value[group.key] = true
  try {
    const result = await testConfigGroup(group.key)
    if (result.success) {
      ElMessage.success(result.message)
      return
    }
    ElMessage.warning(result.details.map(detail => `${detail.target}: ${detail.message}`).join('；'))
  } finally {
    testingGroups.value[group.key] = false
  }
}

const handleImportEnv = async () => {
  importingEnv.value = true
  try {
    const result = await importConfigEnv()
    const imported = result.imported.length
    const failed = Object.keys(result.failed).length
    const skipped = Object.keys(result.skipped).length

    ElMessage.success(`环境变量导入完成：导入 ${imported} 项，跳过 ${skipped} 项，失败 ${failed} 项`)
    await fetchConfigs()
  } finally {
    importingEnv.value = false
  }
}

const handleRunCron = async () => {
  runningCron.value = true
  try {
    const res = await runCronJob()
    ElMessage.success((res as unknown as { message?: string }).message || '任务执行成功')
  } finally {
    runningCron.value = false
  }
}

const visibleValue = (item: AdminConfigItem) => {
  if (item.sensitive) {
    return item.hasValue ? '已设置' : '未设置'
  }
  if (!item.hasValue || item.value === undefined || item.value === '') {
    return '未设置'
  }
  return item.value
}

onMounted(async () => {
  loading.value = true
  try {
    await fetchConfigs()
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="space-y-6 animate-fade-in" v-loading="loading">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">设置中心</h1>
        <p class="mt-1 text-gray-500">统一管理运行期配置、配置来源和部署边界状态</p>
      </div>

      <div class="flex flex-wrap gap-3">
        <button
          type="button"
          @click="fetchConfigs"
          class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50"
        >
          <el-icon><RefreshRight /></el-icon>
          刷新配置
        </button>

        <button
          type="button"
          @click="handleImportEnv"
          :disabled="importingEnv"
          class="inline-flex items-center gap-2 rounded-xl bg-gray-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-black disabled:opacity-70"
        >
          <span
            v-if="importingEnv"
            class="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"
          />
          导入当前环境变量
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm font-medium text-gray-500">已配置项</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ configuredCount }}</p>
        <p class="mt-2 text-xs text-gray-400">当前具有有效值的配置数量</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm font-medium text-gray-500">缺失项</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ missingCount }}</p>
        <p class="mt-2 text-xs text-gray-400">仍需管理员补齐的可编辑配置</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm font-medium text-gray-500">敏感项已设置</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ sensitiveCount }}</p>
        <p class="mt-2 text-xs text-gray-400">密码、密钥等敏感配置状态</p>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <p class="text-sm font-medium text-gray-500">需重启项</p>
        <p class="mt-3 text-3xl font-bold text-gray-900">{{ restartCount }}</p>
        <p class="mt-2 text-xs text-gray-400">部署边界与计划任务相关配置</p>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[240px_minmax(0,1fr)]">
      <aside class="space-y-6">
        <div class="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm">
          <div class="mb-3 flex items-center gap-2 px-2 text-sm font-semibold text-gray-900">
            <el-icon><Setting /></el-icon>
            配置分组
          </div>

          <div class="space-y-2">
            <button
              v-for="group in groupSections"
              :key="group.key"
              type="button"
              @click="activeGroup = group.key"
              class="flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-left text-sm font-medium transition"
              :class="
                activeGroup === group.key
                  ? 'bg-gray-900 text-white'
                  : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              "
            >
              <span>{{ group.label }}</span>
              <span
                class="rounded-full px-2 py-0.5 text-xs"
                :class="activeGroup === group.key ? 'bg-white/15 text-white' : 'bg-gray-100 text-gray-500'"
              >
                {{ group.items.length }}
              </span>
            </button>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm">
          <div class="mb-3 flex items-center gap-2 px-2 text-sm font-semibold text-gray-900">
            <el-icon><Monitor /></el-icon>
            系统维护
          </div>

          <div class="space-y-3">
            <button
              type="button"
              @click="handleRunCron"
              :disabled="runningCron"
              class="w-full rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50 disabled:opacity-70"
            >
              {{ runningCron ? '执行中...' : '立即执行过期检查' }}
            </button>

            <p class="text-xs leading-5 text-gray-400">
              “导入当前环境变量” 只会把允许托管的运行期配置写入数据库，不会改动部署期密钥。
            </p>
          </div>
        </div>
      </aside>

      <main class="space-y-6">
        <section
          v-for="group in groupSections"
          v-show="activeGroup === group.key"
          :key="group.key"
          class="rounded-3xl border border-gray-100 bg-white p-6 shadow-sm"
        >
          <div class="flex flex-col gap-4 border-b border-gray-100 pb-5 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <h2 class="text-xl font-bold text-gray-900">{{ group.label }}</h2>
              <p class="mt-1 text-sm text-gray-500">
                {{
                  group.key === 'deployment'
                    ? '部署期与安全边界配置仅展示状态和来源，不支持在线编辑。'
                    : '按组保存和测试，避免把不同能力混成一个大表单。'
                }}
              </p>
            </div>

            <div class="flex flex-wrap gap-3">
              <button
                v-if="canTestGroup(group.key)"
                type="button"
                @click="handleTestGroup(group)"
                :disabled="testingGroups[group.key]"
                class="rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50 disabled:opacity-70"
              >
                {{ testingGroups[group.key] ? '测试中...' : '测试连接' }}
              </button>

              <button
                v-if="group.items.some(item => item.editable)"
                type="button"
                @click="handleResetGroup(group)"
                class="rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50"
              >
                重置未保存修改
              </button>

              <button
                v-if="group.items.some(item => item.editable)"
                type="button"
                @click="handleSaveGroup(group)"
                :disabled="savingGroups[group.key] || !groupHasChanges(group)"
                class="rounded-xl bg-gray-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-black disabled:cursor-not-allowed disabled:opacity-60"
              >
                {{ savingGroups[group.key] ? '保存中...' : '保存本组配置' }}
              </button>
            </div>
          </div>

          <div class="mt-6 space-y-4">
            <div
              v-for="item in group.items"
              :key="item.key"
              class="rounded-2xl border border-gray-100 bg-gray-50/60 p-5"
            >
              <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <h3 class="text-base font-semibold text-gray-900">{{ item.label }}</h3>
                    <span
                      class="rounded-full px-3 py-1 text-xs font-medium"
                      :class="sourceClass(item.source)"
                    >
                      来源：{{ sourceLabelMap[item.source] }}
                    </span>
                    <span
                      class="rounded-full px-3 py-1 text-xs font-medium"
                      :class="restartClass(item.restartRequired)"
                    >
                      {{ item.restartRequired ? '需重启' : '立即生效' }}
                    </span>
                    <span
                      v-if="item.sensitive"
                      class="rounded-full bg-gray-900/5 px-3 py-1 text-xs font-medium text-gray-700"
                    >
                      {{ item.hasValue ? '敏感项已设置' : '敏感项未设置' }}
                    </span>
                  </div>

                  <p class="mt-2 text-sm leading-6 text-gray-500">{{ item.description }}</p>
                </div>

                <div class="rounded-xl border border-dashed border-gray-200 bg-white px-4 py-3 text-sm text-gray-500 lg:w-72">
                  <p class="font-medium text-gray-700">当前状态</p>
                  <p class="mt-2 break-all">
                    {{ visibleValue(item) }}
                  </p>
                </div>
              </div>

              <div v-if="item.error" class="mt-4 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700">
                {{ item.error }}
              </div>

              <div v-else-if="item.editable" class="mt-4">
                <el-radio-group
                  v-if="item.type === 'enum'"
                  v-model="draftValues[item.key]"
                  class="flex flex-wrap gap-3"
                >
                  <el-radio-button
                    v-for="option in item.options || []"
                    :key="option.value"
                    :label="option.value"
                  >
                    {{ option.label }}
                  </el-radio-button>
                </el-radio-group>

                <div v-else-if="item.type === 'boolean'" class="rounded-2xl border border-gray-200 bg-white p-4">
                  <el-switch
                    v-model="draftValues[item.key]"
                    inline-prompt
                    active-text="开"
                    inactive-text="关"
                  />
                </div>

                <el-input-number
                  v-else-if="item.type === 'integer'"
                  v-model="draftValues[item.key]"
                  :min="0"
                  class="!w-full"
                  controls-position="right"
                />

                <el-checkbox-group
                  v-else-if="item.type === 'json_list'"
                  v-model="draftValues[item.key]"
                  class="flex flex-wrap gap-3 rounded-2xl border border-gray-200 bg-white p-4"
                >
                  <el-checkbox
                    v-for="option in item.options || []"
                    :key="option.value"
                    :label="option.value"
                    class="!mr-0 rounded-xl border border-gray-200 bg-gray-50 px-4 py-2"
                  >
                    {{ option.label }}
                  </el-checkbox>
                </el-checkbox-group>

                <el-input
                  v-else-if="item.sensitive"
                  v-model="draftValues[item.key]"
                  show-password
                  :placeholder="item.hasValue ? '已设置，输入新值以覆盖' : '请输入配置值'"
                  clearable
                />

                <el-input
                  v-else
                  v-model="draftValues[item.key]"
                  :placeholder="item.placeholder || '请输入配置值'"
                  clearable
                />

                <p class="mt-2 text-xs text-gray-400">
                  <template v-if="item.sensitive">
                    敏感值不会回显。只有输入新值时才会覆盖当前值。
                  </template>
                  <template v-else-if="item.source === 'env'">
                    当前正在跟随环境变量。保存后将切换为数据库托管。
                  </template>
                  <template v-else-if="item.restartRequired">
                    该项属于启动期配置，当前页面仅展示状态，不会在线热生效。
                  </template>
                  <template v-else>
                    保存后将立即按当前分组生效。
                  </template>
                </p>
              </div>
            </div>
          </div>
        </section>
      </main>
    </div>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.35s ease-out forwards;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(8px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
