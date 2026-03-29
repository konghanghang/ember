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
import {
  buildConfigUpdatePayload,
  buildDraftValues,
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
      return 'bg-emerald-50 text-emerald-700'
    case 'env':
      return 'bg-amber-50 text-amber-700'
    case 'default':
      return 'bg-sky-50 text-sky-700'
    default:
      return 'bg-gray-100 text-gray-600'
  }
}

const restartClass = (restartRequired: boolean) =>
  restartRequired ? 'bg-rose-50 text-rose-700' : 'bg-emerald-50 text-emerald-700'

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
  if (item.sensitive) {
    return '敏感值不会回显。只有输入新值时才会覆盖当前值。'
  }

  if (hasExplicitEmptyDatabaseValue(item)) {
    return item.emptyValueHint
      ? `${item.emptyValueHint} 当前值来自数据库显式空值。`
      : '当前值来自数据库显式空值。'
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

  return ''
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

const editorPanelClass = (item: AdminConfigItem) => {
  if (item.type === 'enum' || item.type === 'boolean') {
    return 'border-ember/20 bg-ember/5'
  }
  return 'border-gray-100 bg-white'
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
    <section class="rounded-3xl border border-gray-100 bg-white p-5 shadow-sm md:p-6">
      <div class="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
        <div class="space-y-3">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.2em] text-gray-400">Settings Center</p>
            <h1 class="mt-2 text-2xl font-bold text-gray-900">设置中心</h1>
            <p class="mt-1 text-sm text-gray-500">统一管理运行期配置、配置来源和部署边界状态，收口迁移期遗留的展示噪音。</p>
          </div>

          <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-5">
            <div class="rounded-2xl border border-gray-100 bg-gray-50/80 px-4 py-3">
              <p class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">已配置</p>
              <p class="mt-1 text-lg font-semibold text-gray-900">{{ configuredCount }}</p>
            </div>
            <div class="rounded-2xl border border-gray-100 bg-gray-50/80 px-4 py-3">
              <p class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">缺失</p>
              <p class="mt-1 text-lg font-semibold text-gray-900">{{ missingCount }}</p>
            </div>
            <div class="rounded-2xl border border-gray-100 bg-gray-50/80 px-4 py-3">
              <p class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">敏感项</p>
              <p class="mt-1 text-lg font-semibold text-gray-900">{{ sensitiveCount }}</p>
            </div>
            <div class="rounded-2xl border border-gray-100 bg-gray-50/80 px-4 py-3">
              <p class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">需重启</p>
              <p class="mt-1 text-lg font-semibold text-gray-900">{{ restartCount }}</p>
            </div>
            <div
              class="rounded-2xl border px-4 py-3"
              :class="migrationSourceCount > 0 ? 'border-amber-200 bg-amber-50/80' : 'border-gray-100 bg-gray-50/80'"
            >
              <p class="text-[11px] font-semibold uppercase tracking-wide" :class="migrationSourceCount > 0 ? 'text-amber-700' : 'text-gray-400'">
                环境回退
              </p>
              <p class="mt-1 text-lg font-semibold" :class="migrationSourceCount > 0 ? 'text-amber-800' : 'text-gray-900'">
                {{ migrationSourceCount }}
              </p>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap gap-3 lg:max-w-xs lg:justify-end">
          <button
            type="button"
            @click="fetchConfigs"
            class="inline-flex cursor-pointer items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50"
          >
            <el-icon><RefreshRight /></el-icon>
            刷新配置
          </button>

          <button
            type="button"
            @click="handleImportEnv"
            :disabled="importingEnv"
            class="inline-flex cursor-pointer items-center gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-2.5 text-sm font-semibold text-amber-800 transition hover:border-amber-300 hover:bg-amber-100 disabled:opacity-70"
          >
            <span
              v-if="importingEnv"
              class="h-4 w-4 animate-spin rounded-full border-2 border-amber-800/20 border-t-amber-800"
            />
            迁移环境变量
          </button>
        </div>
      </div>
    </section>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[220px_minmax(0,1fr)]">
      <aside class="space-y-4 xl:sticky xl:top-6 xl:self-start">
        <div class="rounded-2xl border border-gray-100 bg-white p-3 shadow-sm">
          <div class="mb-2 flex items-center gap-2 px-2 text-sm font-semibold text-gray-900">
            <el-icon><Setting /></el-icon>
            配置分组
          </div>

          <div class="space-y-1.5">
            <button
              v-for="group in groupSections"
              :key="group.key"
              type="button"
              @click="activeGroup = group.key"
              class="flex w-full cursor-pointer items-center justify-between rounded-xl px-3 py-2.5 text-left text-sm font-medium transition"
              :class="
                activeGroup === group.key
                  ? 'bg-gray-900 text-white shadow-sm'
                  : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              "
            >
              <span class="truncate">{{ group.label }}</span>
              <div class="ml-3 flex items-center gap-2">
                <span
                  v-if="getCriticalMissingItems(group.items).length > 0"
                  class="rounded-full px-2 py-0.5 text-[11px] font-semibold"
                  :class="activeGroup === group.key ? 'bg-red-500/90 text-white' : 'bg-red-100 text-red-700'"
                >
                  {{ getCriticalMissingItems(group.items).length }}
                </span>
                <span
                  class="rounded-full px-2 py-0.5 text-[11px]"
                  :class="activeGroup === group.key ? 'bg-white/15 text-white' : 'bg-gray-100 text-gray-500'"
                >
                  {{ group.items.length }}
                </span>
              </div>
            </button>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm">
          <div class="mb-3 flex items-center gap-2 text-sm font-semibold text-gray-900">
            <el-icon><Monitor /></el-icon>
            系统维护
          </div>

          <button
            type="button"
            @click="handleRunCron"
            :disabled="runningCron"
            class="w-full cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50 disabled:opacity-70"
          >
            {{ runningCron ? '执行中...' : '立即执行过期检查' }}
          </button>

          <p class="mt-3 text-xs leading-5 text-gray-400">
            “迁移环境变量” 只迁运行期配置，不会改部署期密钥。
            <template v-if="migrationSourceCount > 0">
              当前还有 {{ migrationSourceCount }} 项配置仍在跟随环境变量。
            </template>
          </p>
        </div>
      </aside>

      <main>
        <section
          v-if="activeGroupSection"
          class="rounded-3xl border border-gray-100 bg-white p-5 shadow-sm md:p-6"
        >
          <div class="flex flex-col gap-4 border-b border-gray-100 pb-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-xl font-bold text-gray-900">{{ activeGroupSection.label }}</h2>
                <span class="rounded-full bg-gray-100 px-2.5 py-1 text-[11px] font-medium text-gray-600">
                  {{ activeGroupSection.items.length }} 项
                </span>
                <span
                  v-if="activeGroupRiskSummary.count > 0"
                  class="rounded-full bg-red-100 px-2.5 py-1 text-[11px] font-medium text-red-700"
                >
                  {{ activeGroupRiskSummary.count }} 项高风险缺失
                </span>
              </div>
              <p class="mt-1 text-sm text-gray-500">
                {{
                  activeGroupSection.key === 'deployment'
                    ? '部署期与安全边界配置只展示当前状态，不在后台直接编辑。'
                    : '按组保存，减少迁移期兼容信息造成的视觉负担。'
                }}
              </p>
            </div>

            <div class="flex flex-wrap gap-2">
              <button
                v-if="canTestGroup(activeGroupSection.key)"
                type="button"
                @click="handleTestGroup(activeGroupSection)"
                :disabled="testingGroups[activeGroupSection.key]"
                class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50 disabled:opacity-70"
              >
                {{ testingGroups[activeGroupSection.key] ? '测试中...' : '测试连接' }}
              </button>

              <button
                v-if="activeGroupSection.items.some(item => item.editable)"
                type="button"
                @click="handleResetGroup(activeGroupSection)"
                class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50"
              >
                重置未保存修改
              </button>

              <button
                v-if="activeGroupSection.items.some(item => item.editable)"
                type="button"
                @click="handleSaveGroup(activeGroupSection)"
                :disabled="savingGroups[activeGroupSection.key] || !groupHasChanges(activeGroupSection)"
                class="btn-ember rounded-xl px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
              >
                {{ savingGroups[activeGroupSection.key] ? '保存中...' : '保存本组配置' }}
              </button>
            </div>
          </div>

          <div
            v-if="activeGroupRiskSummary.count > 0"
            class="mt-5 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
          >
            <p class="font-semibold text-red-800">高风险缺失</p>
            <p class="mt-1 leading-6">
              当前分组缺少 {{ activeGroupRiskSummary.items.map(item => item.label).join('、') }}，这些项通常需要通过部署环境补齐。
            </p>
          </div>

          <div class="mt-5 overflow-hidden rounded-2xl border border-gray-100 bg-white">
            <div
              v-for="item in activeGroupSection.items"
              :key="item.key"
              class="grid gap-4 px-5 py-5 lg:grid-cols-[220px_minmax(0,1fr)] lg:gap-6"
              :class="[
                !item.hasValue && item.missingValueLevel === 'critical' ? 'bg-red-50/30' : '',
                item.error ? 'bg-red-50/20' : '',
                activeGroupSection.items[activeGroupSection.items.length - 1]?.key !== item.key ? 'border-b border-gray-100' : ''
              ]"
            >
              <div class="space-y-2">
                <div class="flex flex-wrap items-center gap-2">
                  <label class="text-sm font-semibold text-gray-900">{{ item.label }}</label>
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
                <p class="text-sm leading-6 text-gray-500">{{ item.description }}</p>
                <div class="flex flex-wrap gap-2">
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
                </div>
                <p
                  v-if="configStateHint(item)"
                  class="text-xs leading-5"
                  :class="!item.hasValue && item.missingValueLevel === 'critical' ? 'text-red-600' : 'text-gray-400'"
                >
                  {{ configStateHint(item) }}
                </p>
                <p
                  v-if="!item.editable && !item.error"
                  class="text-xs leading-5 text-slate-500"
                >
                  {{ readOnlyHint(item) }}
                </p>
              </div>

              <div class="min-w-0">
                <div
                  v-if="item.error"
                  class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
                >
                  {{ item.error }}
                </div>

                <div
                  v-else-if="item.editable && item.type === 'enum'"
                  class="rounded-xl border border-ember/20 bg-ember/5 p-3"
                >
                  <div class="grid gap-2 sm:grid-cols-2">
                    <button
                      v-for="option in item.options || []"
                      :key="option.value"
                      type="button"
                      class="rounded-xl border px-4 py-3 text-left text-sm font-semibold transition cursor-pointer"
                      :class="
                        draftValues[item.key] === option.value
                          ? 'border-ember/40 bg-white text-ember shadow-sm ring-2 ring-ember/10'
                          : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                      "
                      @click="draftValues[item.key] = option.value"
                    >
                      <span class="block">{{ option.label }}</span>
                      <span
                        v-if="option.value !== option.label"
                        class="mt-1 block text-[11px] font-medium uppercase tracking-wide"
                        :class="draftValues[item.key] === option.value ? 'text-ember/70' : 'text-gray-400'"
                      >
                        {{ option.value }}
                      </span>
                    </button>
                  </div>
                  <p class="mt-2 text-xs leading-5 text-gray-400">
                    {{ editableHint(item) }}
                  </p>
                </div>

                <div
                  v-else-if="item.editable"
                  class="rounded-xl border p-3"
                  :class="editorPanelClass(item)"
                >
                  <div v-if="item.type === 'boolean'" class="rounded-xl border border-gray-200 bg-gray-50 px-4 py-3">
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
                    class="flex flex-wrap gap-2 rounded-xl border border-gray-200 bg-gray-50 p-3"
                  >
                    <el-checkbox
                      v-for="option in item.options || []"
                      :key="option.value"
                      :label="option.value"
                      class="!mr-0 rounded-xl border border-gray-200 bg-white px-3 py-2"
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
                    class="settings-input"
                  />

                  <el-input
                    v-else-if="item.multiline"
                    v-model="draftValues[item.key]"
                    type="textarea"
                    :rows="4"
                    :placeholder="item.placeholder || '请输入配置值'"
                    class="settings-input"
                  />

                  <el-input
                    v-else
                    v-model="draftValues[item.key]"
                    :placeholder="item.placeholder || '请输入配置值'"
                    clearable
                    class="settings-input"
                  />

                  <p class="mt-2 text-xs leading-5 text-gray-400">
                    {{ editableHint(item) }}
                  </p>
                </div>

                <div
                  v-else
                  class="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600"
                >
                  {{ readOnlyHint(item) }}
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
:deep(.settings-input .el-input__wrapper),
:deep(.el-input-number .el-input__wrapper) {
  min-height: 42px;
  border-radius: 0.75rem;
  background-color: #f9fafb !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.settings-input:hover .el-input__wrapper),
:deep(.el-input-number:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.settings-input .el-input__wrapper.is-focus),
:deep(.el-input-number .el-input__wrapper.is-focus),
:deep(.el-input-number.is-focus .el-input__wrapper) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.settings-input .el-textarea__inner) {
  border-radius: 0.75rem;
  background-color: #f9fafb !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  border: none !important;
  transition: all 0.2s ease;
}

:deep(.settings-input:hover .el-textarea__inner) {
  background-color: #ffffff !important;
}

:deep(.settings-input .el-textarea__inner:focus) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

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
