<script setup lang="ts">
import { ref, watch } from 'vue'
import { watchRetentionText } from '@/utils/watch-retention'
import { ElMessage, ElMessageBox } from 'element-plus'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import type { EntitlementAdjustment } from '@/api/entitlements'
import { adjustEntitlement, getEntitlements } from '@/api/entitlements'
import type { ManagedPlanGroup, UserEntitlement } from '@/types/api'
import { formatDateTimeInTimezone } from '@/utils/date'

const props = defineProps<{ userId?: string; groups?: ManagedPlanGroup[]; currentGroup?: string }>()
const emit = defineEmits<{ changed: []; close: [] }>()
const rows = ref<UserEntitlement[]>([])
const loading = ref(false)
const failed = ref(false)
const saving = ref(false)
const group = ref('')
const action = ref('extend')
const operationOptions = [
  { key: 'extend', label: '延长天数' },
  { key: 'set', label: '指定到期时间' },
  { key: 'permanent', label: '设为永久' }
]
const days = ref(30)
const expiresAt = ref('')
const businessTimezone = ref('')
let operationId = ''
let operationPayload = ''

/** 失败时保留重试入口，不将请求失败展示为空权益。 */
async function load() {
  loading.value = true
  failed.value = false
  try { const result = await getEntitlements(props.userId); rows.value = result.data; businessTimezone.value = result.businessTimezone } catch { failed.value = true } finally { loading.value = false }
}

/** 同一失败请求重试复用操作号，编辑操作内容后才生成新号。 */
async function save(revokeGroup?: string) {
  if (!props.userId || saving.value) return
  const payload: Omit<EntitlementAdjustment, 'operationId'> = revokeGroup
    ? { planGroup: revokeGroup, action: 'revoke' }
    : action.value === 'extend'
      ? { planGroup: group.value, action: 'extend', days: days.value }
      : action.value === 'permanent'
        ? { planGroup: group.value, action: 'set', validityType: 'permanent' }
        : { planGroup: group.value, action: 'set', validityType: 'duration', expiresAt: expiresAt.value }
  if (!payload.planGroup) { ElMessage.warning('请选择权益分组'); return }
  if (!revokeGroup && action.value === 'extend' && (!Number.isInteger(days.value) || days.value < 1)) { ElMessage.warning('增加天数必须为正整数'); return }
  if (!revokeGroup && action.value === 'set' && !expiresAt.value) { ElMessage.warning('请选择到期时间'); return }
  if (revokeGroup) {
    try { await ElMessageBox.confirm('撤销该分组权益后，将重新计算当前生效分组。', '撤销权益', { type: 'warning', confirmButtonText: '撤销', cancelButtonText: '取消' }) } catch { return }
  }
  const serialized = JSON.stringify(payload)
  if (!operationId || operationPayload !== serialized) { operationId = Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join(''); operationPayload = serialized }
  saving.value = true
  try {
    await adjustEntitlement(props.userId, { ...payload, operationId })
    operationId = ''
    ElMessage.success('权益已更新')
    await load()
    emit('changed')
  } catch { /* 请求拦截器展示服务端错误；保留操作号用于安全重试。 */ } finally { saving.value = false }
}

watch(() => props.userId, () => { operationId = ''; void load() }, { immediate: true })
</script>

<template>
  <section v-loading="loading" class="space-y-4" :class="userId ? 'p-6 pt-2' : 'rounded-2xl border border-gray-100 bg-white p-4 sm:p-6'">
    <h2 class="text-base font-semibold text-gray-900">持有权益</h2>
    <div v-if="failed" role="alert" class="text-sm text-red-600">权益加载失败 <button class="cursor-pointer underline" @click="load">重试</button></div>
    <p v-else-if="!loading && !rows.length" class="text-sm text-gray-500">暂无权益</p>
    <div v-for="row in rows" :key="row.planGroup" class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gray-100 p-3">
      <div><span class="font-medium">{{ row.planGroupName }}</span><el-tag v-if="row.isCurrent ?? (currentGroup === row.planGroup)" class="ml-2" size="small">当前分组</el-tag>
        <p class="mt-1 text-sm text-gray-500">{{ row.watchRetentionInvalidatedAt ? '观看要求未达标，权益已失效' : row.validityType === 'permanent' ? '永久有效' : `到期时间：${formatDateTimeInTimezone(row.expiresAt || '', businessTimezone)}` }}</p>
        <p v-if="row.watchRetention" class="mt-1 text-sm text-gray-500">{{ watchRetentionText(row.watchRetention, businessTimezone) }}</p>
      </div>
      <button v-if="userId" :disabled="saving" class="cursor-pointer rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600 disabled:opacity-50" @click="save(row.planGroup)">撤销</button>
    </div>
    <el-form v-if="userId" label-position="top" class="border-t border-gray-100 pt-4" @submit.prevent="save()">
      <el-form-item label="目标权益分组"><el-select v-model="group" class="form-select w-full"><el-option v-for="item in groups" :key="item.key" :label="item.name" :value="item.key" /></el-select></el-form-item>
      <el-form-item label="操作">
        <EmberSegmentTabs v-model="action" :tabs="operationOptions" ariaLabel="权益调整方式" />
      </el-form-item>
      <el-form-item v-if="action === 'extend'" label="增加天数"><el-input-number v-model="days" :min="1" :precision="0" class="form-number" /></el-form-item>
      <el-form-item v-if="action === 'set'" :label="`到期时间（${businessTimezone}）`">
        <el-date-picker v-model="expiresAt" type="datetime" value-format="YYYY-MM-DD HH:mm:ss" placeholder="选择到期时间" class="form-date w-full" />
      </el-form-item>
      <div class="mt-6 flex justify-end gap-3 border-t border-gray-100 pt-4">
        <button type="button" class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-50" @click="emit('close')">取消</button>
        <button type="submit" class="btn-ember cursor-pointer rounded-xl px-6 py-2.5 text-sm font-semibold shadow-sm disabled:cursor-not-allowed disabled:opacity-50" :disabled="saving || loading || failed">{{ saving ? '保存中…' : '保存权益' }}</button>
      </div>
    </el-form>
  </section>
</template>
