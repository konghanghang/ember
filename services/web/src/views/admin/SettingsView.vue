<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Monitor, RefreshRight, Setting } from '@element-plus/icons-vue'
import {
  getConfigs,
  importConfigEnv,
  runCronJob,
  testConfigGroup,
  updateConfig
} from '@/api/admin'
import type { AdminConfigItem } from '@/types/api'
import {
  buildConfigUpdatePayload,
  buildDraftValues,
  canClearConfigOverride,
  getClearConfigDescription,
  getClearConfigLabel,
  hasExplicitEmptyDatabaseValue,
  isConfigItemDirty,
  parseDraftValue
} from './settings-center.utils'

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

interface ConfigRiskSummary {
  count: number
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
const clearingItems = ref<Record<string, boolean>>({})

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

const activeGroupSection = computed(() => groupSections.value.find(group => group.key === activeGroup.value) ?? null)

const configuredCount = computed(() => configs.value.filter(item => item.hasValue).length)
const missingCount = computed(() => configs.value.filter(item => !item.hasValue && item.editable).length)
const sensitiveCount = computed(() => configs.value.filter(item => item.sensitive && item.hasValue).length)
const restartCount = computed(() => configs.value.filter(item => item.restartRequired).length)

const getCriticalMissingItems = (items: AdminConfigItem[]) =>
  items.filter(item => !item.hasValue && item.missingValueLevel === 'critical')

const activeGroupRiskSummary = computed<ConfigRiskSummary>(() => {
  const items = activeGroupSection.value ? getCriticalMissingItems(activeGroupSection.value.items) : []
  return {
    count: items.length,
    items
  }
})

const migrationSourceCount = computed(() =>
  configs.value.filter(item => item.editable && item.source === 'env').length
)

const resetDraftValues = (items: AdminConfigItem[]) => {
  draftValues.value = buildDraftValues(items)
}

const resetDraftValue = (item: AdminConfigItem) => {
  draftValues.value[item.key] = parseDraftValue(item)
}

const applyUpdatedConfigItem = (updatedItem: AdminConfigItem) => {
  configs.value = configs.value.map(item => (item.key === updatedItem.key ? updatedItem : item))
  resetDraftValue(updatedItem)
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

const isItemDirty = (item: AdminConfigItem) => isConfigItemDirty(item, draftValues.value[item.key])

const groupHasChanges = (group: ConfigGroupSection) =>
  group.items.some(item => item.editable && isItemDirty(item))

const buildUpdatePayload = (item: AdminConfigItem) =>
  buildConfigUpdatePayload(item, draftValues.value[item.key])

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
    draftValues.value[item.key] = parseDraftValue(item)
  }

  ElMessage.success(`${group.label}已恢复到当前生效值`)
}

const handleClearConfigOverride = async (item: AdminConfigItem) => {
  const label = getClearConfigLabel(item)

  try {
    await ElMessageBox.confirm(
      `${getClearConfigDescription(item)}\n\n配置项：${item.label}`,
      label,
      {
        confirmButtonText: '确认清空',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }

  clearingItems.value[item.key] = true
  try {
    const updatedItem = await updateConfig(item.key, { clear: true })
    applyUpdatedConfigItem(updatedItem)
    ElMessage.success(`${item.label}已移除数据库覆盖值`)
  } finally {
    clearingItems.value[item.key] = false
  }
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

    ElMessage.success(`环境变量迁移完成：导入 ${imported} 项，跳过 ${skipped} 项，失败 ${failed} 项`)
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

const configStateHint = (item: AdminConfigItem) => {
  if (hasExplicitEmptyDatabaseValue(item) && item.emptyValueHint) {
    return item.emptyValueHint
  }
  if (!item.hasValue && item.missingValueHint) {
    return item.missingValueHint
  }
  return ''
}

const editableHint = (item: AdminConfigItem) => {
  const fallbackHint = item.fallbackHint || '移除数据库覆盖值后将按系统规则回退。'

  if (item.sensitive) {
    return '敏感值不会回显。只有输入新值时才会覆盖当前值。'
  }

  if (hasExplicitEmptyDatabaseValue(item)) {
    return item.emptyValueHint
      ? `${item.emptyValueHint} 也可使用“${getClearConfigLabel(item)}”。${fallbackHint}`
      : `当前值来自数据库显式空值。可使用“${getClearConfigLabel(item)}”。${fallbackHint}`
  }

  if (item.allowEmpty && item.emptyValueHint && canClearConfigOverride(item)) {
    return `可保存为空值。${item.emptyValueHint} 也可使用“${getClearConfigLabel(item)}”。${fallbackHint}`
  }

  if (canClearConfigOverride(item)) {
    return `当前值来自数据库覆盖。可使用“${getClearConfigLabel(item)}”。${fallbackHint}`
  }

  if (item.allowEmpty && item.emptyValueHint) {
    return `可保存为空值。${item.emptyValueHint}`
  }

  if (item.source === 'env') {
    return '当前正在跟随环境变量。保存后将切换为数据库托管；后续以设置中心为准。'
  }

  if (item.restartRequired) {
    return '该项属于启动期配置，当前页面仅展示状态，不会在线热生效。'
  }

  return '保存后将立即按当前分组生效。'
}

const readOnlyHint = (item: AdminConfigItem) => {
  if (item.editable) {
    return ''
  }

  if (item.readOnlyHint) {
    return item.readOnlyHint
  }

  if (item.restartRequired) {
    return '该项属于部署期或启动期边界配置，只做状态展示，修改需要通过部署环境完成并重启对应服务。'
  }

  return '该项当前只做状态展示，不支持在后台在线编辑。'
}

const riskBadgeText = (item: AdminConfigItem) => {
  if (item.hasValue || item.missingValueLevel === 'none') {
    return ''
  }
  switch (item.missingValueLevel) {
    case 'critical':
      return '高风险缺失'
    case 'warning':
      return '风险提示'
    case 'info':
      return '配置提示'
    default:
      return ''
  }
}

const riskBadgeClass = (item: AdminConfigItem) => {
  switch (item.missingValueLevel) {
    case 'critical':
      return 'bg-red-100 text-red-700'
    case 'warning':
      return 'bg-amber-100 text-amber-700'
    case 'info':
      return 'bg-sky-100 text-sky-700'
    default:
      return 'bg-gray-100 text-gray-600'
  }
}

const itemStatusSummary = (item: AdminConfigItem) => {
  if (item.sensitive) {
    return item.hasValue ? '已设置敏感值' : '敏感值未设置'
  }
  if (hasExplicitEmptyDatabaseValue(item)) {
    return '数据库显式空值'
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
          class="inline-flex items-center gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-2 text-sm font-semibold text-amber-800 transition hover:border-amber-300 hover:bg-amber-100 disabled:opacity-70"
        >
          <span
            v-if="importingEnv"
            class="h-4 w-4 animate-spin rounded-full border-2 border-amber-800/20 border-t-amber-800"
          />
          迁移环境变量
        </button>
      </div>
    </div>

    <div class="flex flex-wrap gap-3 rounded-2xl border border-gray-100 bg-white px-4 py-3 shadow-sm">
      <div class="rounded-xl bg-gray-100 px-3 py-2 text-sm text-gray-700">
        已配置 <span class="ml-1 font-semibold text-gray-900">{{ configuredCount }}</span>
      </div>
      <div class="rounded-xl bg-gray-100 px-3 py-2 text-sm text-gray-700">
        缺失 <span class="ml-1 font-semibold text-gray-900">{{ missingCount }}</span>
      </div>
      <div class="rounded-xl bg-gray-100 px-3 py-2 text-sm text-gray-700">
        敏感项已设置 <span class="ml-1 font-semibold text-gray-900">{{ sensitiveCount }}</span>
      </div>
      <div class="rounded-xl bg-gray-100 px-3 py-2 text-sm text-gray-700">
        需重启 <span class="ml-1 font-semibold text-gray-900">{{ restartCount }}</span>
      </div>
      <div
        v-if="migrationSourceCount > 0"
        class="rounded-xl bg-amber-50 px-3 py-2 text-sm text-amber-800"
      >
        跟随环境变量 <span class="ml-1 font-semibold">{{ migrationSourceCount }}</span>
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
              <div class="flex items-center gap-2">
                <span
                  v-if="getCriticalMissingItems(group.items).length > 0"
                  class="rounded-full px-2 py-0.5 text-[11px] font-semibold"
                  :class="activeGroup === group.key ? 'bg-red-500/90 text-white' : 'bg-red-100 text-red-700'"
                >
                  {{ getCriticalMissingItems(group.items).length }} 风险
                </span>
                <span
                  class="rounded-full px-2 py-0.5 text-xs"
                  :class="activeGroup === group.key ? 'bg-white/15 text-white' : 'bg-gray-100 text-gray-500'"
                >
                  {{ group.items.length }}
                </span>
              </div>
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
              “迁移环境变量” 是一次性迁移工具，只会把允许托管的运行期配置写入数据库，不会改动部署期密钥。
              <template v-if="migrationSourceCount > 0">
                当前仍有 {{ migrationSourceCount }} 项配置正在跟随环境变量。
              </template>
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

          <div
            v-if="activeGroupRiskSummary.count > 0"
            class="mt-6 rounded-2xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-700"
          >
            <p class="font-semibold text-red-800">高风险缺失</p>
            <p class="mt-2 leading-6">
              当前分组有 {{ activeGroupRiskSummary.count }} 项关键边界配置缺失：
              {{ activeGroupRiskSummary.items.map(item => item.label).join('、') }}。
              这些项通常需要通过部署环境补齐，否则对应能力无法正常工作。
            </p>
          </div>

          <div class="mt-6 space-y-3">
            <div
              v-for="item in group.items"
              :key="item.key"
              class="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm"
            >
              <div class="grid gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(320px,1.2fr)_220px] xl:items-start">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <h3 class="text-sm font-semibold text-gray-900">{{ item.label }}</h3>
                    <span
                      v-if="!item.editable"
                      class="rounded-full bg-slate-100 px-2.5 py-1 text-[11px] font-medium text-slate-700"
                    >
                      只读
                    </span>
                    <span
                      v-if="riskBadgeText(item)"
                      class="rounded-full px-2.5 py-1 text-[11px] font-medium"
                      :class="riskBadgeClass(item)"
                    >
                      {{ riskBadgeText(item) }}
                    </span>
                  </div>

                  <p class="mt-2 text-sm font-medium text-gray-900 break-all">
                    {{ itemStatusSummary(item) }}
                  </p>
                  <p class="mt-1 text-xs leading-5 text-gray-500">
                    {{ item.description }}
                  </p>
                  <p
                    v-if="configStateHint(item)"
                    class="mt-1 text-xs leading-5"
                    :class="!item.hasValue && item.missingValueLevel === 'critical' ? 'text-red-600' : 'text-gray-400'"
                  >
                    {{ configStateHint(item) }}
                  </p>
                </div>

                <div v-if="item.error" class="rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700">
                  {{ item.error }}
                </div>

                <div v-else-if="item.editable">
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

                  <div v-else-if="item.type === 'boolean'" class="rounded-2xl border border-gray-200 bg-gray-50 p-4">
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
                    :min="item.minValue"
                    :max="item.maxValue"
                    class="!w-full"
                    controls-position="right"
                  />

                  <el-checkbox-group
                    v-else-if="item.type === 'json_list'"
                    v-model="draftValues[item.key]"
                    class="flex flex-wrap gap-3 rounded-2xl border border-gray-200 bg-gray-50 p-4"
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

                  <p class="mt-2 text-xs leading-5 text-gray-400">
                    {{ editableHint(item) }}
                  </p>
                </div>

                <div v-else class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
                  <p class="font-medium text-slate-800">只读原因</p>
                  <p class="mt-2 leading-6">
                    {{ readOnlyHint(item) }}
                  </p>
                  <p v-if="item.restartRequired" class="mt-2 text-xs text-slate-500">
                    如需修改，请更新部署环境并重启对应服务后再回到此页确认状态。
                  </p>
                </div>

                <div class="flex flex-col items-start gap-2 xl:items-end">
                  <span
                    class="rounded-full px-2.5 py-1 text-[11px] font-medium"
                    :class="sourceClass(item.source)"
                  >
                    来源：{{ sourceLabelMap[item.source] }}
                  </span>
                  <span
                    class="rounded-full px-2.5 py-1 text-[11px] font-medium"
                    :class="restartClass(item.restartRequired)"
                  >
                    {{ item.restartRequired ? '需重启' : '立即生效' }}
                  </span>
                  <span
                    v-if="item.sensitive"
                    class="rounded-full bg-gray-900/5 px-2.5 py-1 text-[11px] font-medium text-gray-700"
                  >
                    {{ item.hasValue ? '敏感项已设置' : '敏感项未设置' }}
                  </span>
                  <button
                    v-if="canClearConfigOverride(item)"
                    type="button"
                    @click="handleClearConfigOverride(item)"
                    :disabled="clearingItems[item.key]"
                    class="mt-1 text-xs font-medium text-amber-700 transition hover:text-amber-800 disabled:opacity-60"
                  >
                    {{ clearingItems[item.key] ? '处理中...' : '恢复回退' }}
                  </button>
                </div>
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
