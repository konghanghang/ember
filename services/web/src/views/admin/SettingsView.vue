<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Key, Monitor, QuestionFilled, Setting } from '@element-plus/icons-vue'
import EmberMetricCard from '@/components/ember/data-display/EmberMetricCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import {
  deleteExternalApiKey,
  generateExternalApiKey,
  getConfigs,
  getExternalApiKeyStatus,
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
const runningCron = ref(false)
const activeGroup = ref<ConfigGroupKey>('business')
const configs = ref<AdminConfigItem[]>([])
const draftValues = ref<Record<string, any>>({})
const savingGroups = ref<Record<string, boolean>>({})
const testingGroups = ref<Record<string, boolean>>({})
const groupTabRefs = ref<Partial<Record<ConfigGroupKey, HTMLButtonElement | null>>>({})
const apiKeyConfigured = ref(false)
const apiKeyMutating = ref(false)
const generatedApiKey = ref('')
const apiKeyDialogVisible = ref(false)

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
const groupKeys = computed(() => groupSections.value.map(group => group.key))

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

const fetchExternalApiKeyStatus = async () => {
  const res = await getExternalApiKeyStatus()
  apiKeyConfigured.value = res.data.configured
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

const handleRunCron = async () => {
  runningCron.value = true
  try {
    const res = await runCronJob()
    ElMessage.success((res as unknown as { message?: string }).message || '任务执行成功')
  } finally {
    runningCron.value = false
  }
}

const copyApiKey = async () => {
  if (!generatedApiKey.value) return

  try {
    await navigator.clipboard.writeText(generatedApiKey.value)
    ElMessage.success('复制成功')
  } catch {
    ElMessage.error('复制失败')
  }
}

const handleGenerateApiKey = async () => {
  if (apiKeyConfigured.value) {
    try {
      await ElMessageBox.confirm(
        '重新生成会立即替换旧 Key，旧 Key 将失效。',
        '重新生成 Admin API Key',
        {
          confirmButtonText: '重新生成',
          cancelButtonText: '取消',
          type: 'warning',
        }
      )
    } catch {
      return
    }
  }

  apiKeyMutating.value = true
  try {
    const res = await generateExternalApiKey()
    apiKeyConfigured.value = res.data.configured
    generatedApiKey.value = res.data.apiKey
    apiKeyDialogVisible.value = true
  } finally {
    apiKeyMutating.value = false
  }
}

const handleDisableApiKey = async () => {
  try {
    await ElMessageBox.confirm(
      '禁用后，所有正在使用该 Key 的外部脚本会立即失效。',
      '禁用 Admin API Key',
      {
        confirmButtonText: '禁用',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      }
    )
  } catch {
    return
  }

  apiKeyMutating.value = true
  try {
    const res = await deleteExternalApiKey()
    apiKeyConfigured.value = res.data.configured
    generatedApiKey.value = ''
    apiKeyDialogVisible.value = false
    ElMessage.success('Admin API Key 已禁用')
  } finally {
    apiKeyMutating.value = false
  }
}

const handleApiKeyDialogClosed = () => {
  generatedApiKey.value = ''
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
    if (item.maskedValue) {
      return `当前值以脱敏形式显示为 ${item.maskedValue}。只有输入新值时才会覆盖当前值。`
    }
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

const shouldShowInlineStateHint = (item: AdminConfigItem) =>
  Boolean(configStateHint(item) && !item.hasValue && item.missingValueLevel === 'critical')

const itemTooltipSections = (item: AdminConfigItem) => {
  const sections: Array<{ label: string; text: string }> = []

  if (item.description) {
    sections.push({
      label: '字段说明',
      text: item.description,
    })
  }

  const stateHint = configStateHint(item)
  if (stateHint && !shouldShowInlineStateHint(item)) {
    sections.push({
      label: '状态提示',
      text: stateHint,
    })
  }

  const modeHint = item.editable ? editableHint(item) : readOnlyHint(item)
  if (modeHint) {
    sections.push({
      label: item.editable ? '编辑提示' : '只读原因',
      text: modeHint,
    })
  }

  return sections
}

const compactReadOnlySummary = (item: AdminConfigItem) =>
  item.restartRequired ? '仅展示当前状态' : '后台不可编辑'

const setGroupTabRef = (groupKey: ConfigGroupKey, element: Element | null) => {
  groupTabRefs.value[groupKey] = element instanceof HTMLButtonElement ? element : null
}

const focusGroupTab = (groupKey: ConfigGroupKey) => {
  nextTick(() => {
    groupTabRefs.value[groupKey]?.focus()
  })
}

const activateGroup = (groupKey: ConfigGroupKey, options?: { focus?: boolean }) => {
  activeGroup.value = groupKey
  if (options?.focus) {
    focusGroupTab(groupKey)
  }
}

const moveGroupFocus = (currentKey: ConfigGroupKey, offset: number) => {
  const keys = groupKeys.value
  const currentIndex = keys.indexOf(currentKey)
  if (currentIndex === -1 || keys.length === 0) return
  const nextIndex = (currentIndex + offset + keys.length) % keys.length
  activateGroup(keys[nextIndex], { focus: true })
}

const jumpGroupFocus = (target: 'first' | 'last') => {
  const keys = groupKeys.value
  if (keys.length === 0) return
  activateGroup(target === 'first' ? keys[0] : keys[keys.length - 1], { focus: true })
}

const handleGroupTabKeydown = (event: KeyboardEvent, groupKey: ConfigGroupKey) => {
  switch (event.key) {
    case 'ArrowDown':
    case 'ArrowRight':
      event.preventDefault()
      moveGroupFocus(groupKey, 1)
      break
    case 'ArrowUp':
    case 'ArrowLeft':
      event.preventDefault()
      moveGroupFocus(groupKey, -1)
      break
    case 'Home':
      event.preventDefault()
      jumpGroupFocus('first')
      break
    case 'End':
      event.preventDefault()
      jumpGroupFocus('last')
      break
    default:
      break
  }
}

onMounted(async () => {
  loading.value = true
  try {
    await Promise.all([fetchConfigs(), fetchExternalApiKeyStatus()])
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="space-y-5 animate-fade-in" v-loading="loading">
    <EmberPageHeaderCard title="设置中心" description="统一管理运行期配置、配置来源和部署边界状态，优先突出当前状态和可执行操作。">
      <template #titleSuffix>
        <span class="rounded-full border border-gray-200 bg-gray-50 px-2.5 py-1 text-[11px] font-medium text-gray-600">
          {{ groupSections.length }} 个分组
        </span>
      </template>

      <div class="mt-2">
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-gray-400">Settings Center</p>
      </div>

      <div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <EmberMetricCard title="已配置" :value="configuredCount" value-class="mt-1 text-lg font-semibold text-gray-900" />
        <EmberMetricCard title="缺失" :value="missingCount" value-class="mt-1 text-lg font-semibold text-gray-900" />
        <EmberMetricCard title="敏感项" :value="sensitiveCount" value-class="mt-1 text-lg font-semibold text-gray-900" />
        <EmberMetricCard title="需重启" :value="restartCount" value-class="mt-1 text-lg font-semibold text-gray-900" />
      </div>
    </EmberPageHeaderCard>

    <div class="grid grid-cols-1 gap-5 xl:grid-cols-[208px_minmax(0,1fr)]">
      <aside class="space-y-3 xl:sticky xl:top-6 xl:self-start">
        <div class="rounded-2xl border border-gray-100 bg-white p-3 shadow-sm">
          <div class="mb-2 flex items-center gap-2 px-2 text-sm font-semibold text-gray-900">
            <el-icon><Setting /></el-icon>
            配置分组
          </div>

          <div class="space-y-1.5" role="tablist" aria-label="设置分组" aria-orientation="vertical">
            <button
              v-for="group in groupSections"
              :key="group.key"
              :ref="(element) => setGroupTabRef(group.key, element)"
              type="button"
              :id="`settings-tab-${group.key}`"
              :aria-selected="activeGroup === group.key"
              :aria-controls="`settings-panel-${group.key}`"
              :tabindex="activeGroup === group.key ? 0 : -1"
              role="tab"
              @click="activateGroup(group.key)"
              @keydown="handleGroupTabKeydown($event, group.key)"
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

        <div class="rounded-2xl border border-gray-100 bg-white p-3.5 shadow-sm">
          <div class="mb-2.5 flex items-center gap-2 text-sm font-semibold text-gray-900">
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

          <p class="mt-2.5 text-xs leading-5 text-gray-400">
            这里只保留当前系统真正需要的维护动作；配置修改以设置中心保存结果为准。
          </p>
        </div>
      </aside>

      <main>
        <section
          v-if="activeGroupSection"
          :id="`settings-panel-${activeGroupSection.key}`"
          :aria-labelledby="`settings-tab-${activeGroupSection.key}`"
          role="tabpanel"
          class="rounded-3xl border border-gray-100 bg-white p-4 shadow-sm md:p-5"
        >
          <div class="flex flex-col gap-3 border-b border-gray-100 pb-3 lg:flex-row lg:items-start lg:justify-between">
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
              <p
                v-if="activeGroupSection.key === 'deployment'"
                class="mt-1 text-sm leading-6 text-gray-500"
              >
                部署期与安全边界配置只展示当前状态，不在后台直接编辑。
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
            class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
          >
            <p class="font-semibold text-red-800">高风险缺失</p>
            <p class="mt-1 leading-6">
              当前分组缺少 {{ activeGroupRiskSummary.items.map(item => item.label).join('、') }}，这些项通常需要通过部署环境补齐。
            </p>
          </div>

          <div
            v-if="activeGroupSection.key === 'deployment'"
            class="mt-4 border-b border-gray-100 pb-4"
          >
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="inline-flex h-9 w-9 items-center justify-center rounded-xl bg-gray-900 text-white">
                    <el-icon><Key /></el-icon>
                  </span>
                  <div>
                    <h3 class="text-base font-bold text-gray-900">Admin API Key</h3>
                    <p class="mt-0.5 text-sm text-gray-500">生成后只展示一次明文，请立即复制保存。</p>
                  </div>
                </div>
              </div>

              <div class="flex flex-wrap items-center gap-2">
                <span
                  class="rounded-full px-2.5 py-1 text-xs font-semibold"
                  :class="apiKeyConfigured ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'"
                >
                  {{ apiKeyConfigured ? '已启用' : '未启用' }}
                </span>
                <button
                  type="button"
                  @click="handleGenerateApiKey"
                  :disabled="apiKeyMutating"
                  class="btn-ember inline-flex cursor-pointer items-center gap-1.5 rounded-xl px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <el-icon><Key /></el-icon>
                  {{ apiKeyMutating ? '处理中...' : (apiKeyConfigured ? '重新生成' : '生成') }}
                </button>
                <button
                  type="button"
                  @click="handleDisableApiKey"
                  :disabled="apiKeyMutating || !apiKeyConfigured"
                  class="inline-flex cursor-pointer items-center rounded-xl border border-red-200 bg-white px-4 py-2 text-sm font-semibold text-red-600 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  禁用
                </button>
              </div>
            </div>
          </div>

          <div class="mt-4 overflow-hidden rounded-2xl border border-gray-100 bg-white">
            <div
              v-for="item in activeGroupSection.items"
              :key="item.key"
              class="px-4 py-4 lg:px-5"
              :class="[
                !item.hasValue && item.missingValueLevel === 'critical' ? 'bg-red-50/30' : '',
                item.error ? 'bg-red-50/20' : '',
                activeGroupSection.items[activeGroupSection.items.length - 1]?.key !== item.key ? 'border-b border-gray-100' : ''
              ]"
            >
              <div class="flex flex-col gap-3 lg:grid lg:grid-cols-[220px_minmax(0,1fr)] lg:items-start lg:gap-4">
                <div class="min-w-0 space-y-1.5">
                  <div class="flex flex-wrap items-center gap-2">
                    <label class="text-sm font-semibold text-gray-900">{{ item.label }}</label>
                    <el-tooltip
                      v-if="itemTooltipSections(item).length > 0"
                      placement="top-start"
                      effect="light"
                      :show-after="150"
                    >
                      <template #content>
                        <div class="max-w-[280px] space-y-2 text-xs leading-5">
                          <div
                            v-for="section in itemTooltipSections(item)"
                            :key="`${item.key}-${section.label}`"
                          >
                            <p class="font-semibold text-gray-700">{{ section.label }}</p>
                            <p class="mt-0.5 text-gray-600">{{ section.text }}</p>
                          </div>
                        </div>
                      </template>
                      <button
                        type="button"
                        class="inline-flex h-5 w-5 cursor-help items-center justify-center rounded-full border border-gray-200 bg-white text-[11px] text-gray-400 transition hover:border-gray-300 hover:text-gray-600"
                        :aria-label="`查看${item.label}说明`"
                      >
                        <el-icon><QuestionFilled /></el-icon>
                      </button>
                    </el-tooltip>
                  </div>

                  <div class="flex flex-wrap gap-2">
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
                    v-if="shouldShowInlineStateHint(item)"
                    class="text-xs leading-5"
                    :class="!item.hasValue && item.missingValueLevel === 'critical' ? 'text-red-600' : 'text-gray-400'"
                  >
                    {{ configStateHint(item) }}
                  </p>
                </div>

                <div class="min-w-0 w-full">
                  <div
                    v-if="item.error"
                    class="rounded-xl border border-red-200 bg-red-50 px-3.5 py-3 text-sm text-red-700"
                  >
                    {{ item.error }}
                  </div>

                  <div
                    v-else-if="item.editable && item.type === 'enum'"
                    class="rounded-xl border border-gray-200 bg-gray-50 p-1.5"
                  >
                    <div class="flex flex-wrap gap-1.5">
                      <button
                        v-for="option in item.options || []"
                        :key="option.value"
                        type="button"
                        class="inline-flex cursor-pointer items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium leading-5 transition"
                        :class="
                          draftValues[item.key] === option.value
                            ? 'border-ember/30 bg-white text-ember shadow-sm ring-2 ring-ember/10'
                            : 'border-transparent bg-transparent text-gray-600 hover:border-gray-200 hover:bg-white hover:text-gray-900'
                        "
                        @click="draftValues[item.key] = option.value"
                      >
                        <span>{{ option.label }}</span>
                        <span
                          v-if="option.value !== option.label"
                          class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                          :class="
                            draftValues[item.key] === option.value
                              ? 'bg-ember/10 text-ember/80'
                              : 'bg-gray-200/80 text-gray-400'
                          "
                        >
                          {{ option.value }}
                        </span>
                      </button>
                    </div>
                  </div>

                  <div
                    v-else-if="item.editable && item.type === 'boolean'"
                    class="rounded-xl border border-ember/20 bg-ember/5 p-2.5"
                  >
                    <div class="rounded-xl border border-gray-200 bg-gray-50 px-3.5 py-2.5">
                      <el-switch
                        v-model="draftValues[item.key]"
                        inline-prompt
                        active-text="开"
                        inactive-text="关"
                      />
                    </div>
                  </div>

                  <div
                    v-else-if="item.editable && item.type === 'json_list'"
                    class="rounded-xl border border-gray-100 bg-white p-2.5"
                  >
                    <el-checkbox-group
                      v-model="draftValues[item.key]"
                      class="flex flex-wrap gap-2 rounded-xl border border-gray-200 bg-gray-50 p-2.5"
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
                  </div>

                  <el-input-number
                    v-else-if="item.editable && item.type === 'integer'"
                    v-model="draftValues[item.key]"
                    :min="item.minValue"
                    :max="item.maxValue"
                    class="w-full !w-full form-number"
                    controls-position="right"
                  />

                  <el-input
                    v-else-if="item.editable && item.sensitive"
                    v-model="draftValues[item.key]"
                    show-password
                    :placeholder="item.hasValue ? (item.maskedValue ? `${item.maskedValue}（输入新值以覆盖）` : '已设置，输入新值以覆盖') : '请输入配置值'"
                    clearable
                    class="input-ember"
                  />

                  <el-input
                    v-else-if="item.editable && item.multiline"
                    v-model="draftValues[item.key]"
                    type="textarea"
                    :rows="3"
                    :placeholder="item.placeholder || '请输入配置值'"
                    class="input-ember"
                  />

                  <el-input
                    v-else-if="item.editable"
                    v-model="draftValues[item.key]"
                    :placeholder="item.placeholder || '请输入配置值'"
                    clearable
                    class="input-ember"
                  />

                  <div
                    v-else
                    class="rounded-xl border border-slate-200 bg-slate-50 px-3.5 py-2.5 text-sm text-slate-600"
                  >
                    <p class="text-[11px] font-semibold uppercase tracking-wide text-slate-500">只读边界</p>
                    <p class="mt-0.5 leading-5">{{ compactReadOnlySummary(item) }}</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </main>
    </div>

    <el-dialog
      v-model="apiKeyDialogVisible"
      title="Admin API Key"
      width="560px"
      :destroy-on-close="true"
      @closed="handleApiKeyDialogClosed"
    >
      <div class="space-y-3">
        <p class="text-sm leading-6 text-gray-600">
          这是唯一一次明文展示，关闭后无法再次查看。
        </p>
        <div class="rounded-2xl border border-gray-200 bg-gray-50 p-3">
          <code class="block break-all font-mono text-sm leading-6 text-gray-900">{{ generatedApiKey }}</code>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            @click="copyApiKey"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-semibold text-gray-700 transition hover:bg-gray-50"
          >
            <el-icon><CopyDocument /></el-icon>
            复制
          </button>
          <button
            type="button"
            @click="apiKeyDialogVisible = false"
            class="btn-ember cursor-pointer rounded-xl px-4 py-2 text-sm font-semibold"
          >
            我已保存
          </button>
        </div>
      </template>
    </el-dialog>
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
