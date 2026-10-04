<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { adjustEntitlement, getEntitlements } from '@/api/entitlements'
import type { ManagedPlanGroup, UserEntitlement } from '@/types/api'
import { formatDateTimeInTimezone } from '@/utils/date'

const props = defineProps<{ userId?: string; groups?: ManagedPlanGroup[]; currentGroup?: string }>()
const emit = defineEmits<{ changed: [] }>()
const rows = ref<UserEntitlement[]>([])
const loading = ref(false)
const failed = ref(false)
const saving = ref(false)
const group = ref('')
const action = ref<'set' | 'extend'>('extend')
const validityType = ref<'duration' | 'permanent'>('duration')
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
  const payload = revokeGroup ? { planGroup: revokeGroup, action: 'revoke' as const } : {
    planGroup: group.value, action: action.value, validityType: validityType.value,
    days: days.value, expiresAt: action.value === 'set' && validityType.value === 'duration' ? expiresAt.value : null
  }
  if (!payload.planGroup) { ElMessage.warning('请选择权益分组'); return }
  if (!revokeGroup && action.value === 'set' && validityType.value === 'duration' && !expiresAt.value) { ElMessage.warning('请选择到期时间'); return }
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
  <section v-loading="loading" class="space-y-4 rounded-2xl border border-gray-100 bg-white p-4 sm:p-6">
    <h2 class="text-base font-semibold text-gray-900">持有权益</h2>
    <div v-if="failed" role="alert" class="text-sm text-red-600">权益加载失败 <button class="cursor-pointer underline" @click="load">重试</button></div>
    <p v-else-if="!loading && !rows.length" class="text-sm text-gray-500">暂无权益</p>
    <div v-for="row in rows" :key="row.planGroup" class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gray-100 p-3">
      <div><span class="font-medium">{{ row.planGroupName }}</span><el-tag v-if="currentGroup === row.planGroup" class="ml-2" size="small">当前分组</el-tag>
        <p class="mt-1 text-sm text-gray-500">{{ row.validityType === 'permanent' ? '永久有效' : `到期时间：${formatDateTimeInTimezone(row.expiresAt || '', businessTimezone)}` }}</p>
      </div>
      <button v-if="userId" :disabled="saving" class="cursor-pointer rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600 disabled:opacity-50" @click="save(row.planGroup)">撤销</button>
    </div>
    <el-form v-if="userId" label-position="top" class="border-t border-gray-100 pt-4" @submit.prevent="save()">
      <el-form-item label="目标权益分组"><el-select v-model="group" class="form-select w-full"><el-option v-for="item in groups" :key="item.key" :label="item.name" :value="item.key" /></el-select></el-form-item>
      <el-form-item label="操作"><el-select v-model="action" class="form-select w-full"><el-option label="延长有效期" value="extend" /><el-option label="设置有效期" value="set" /></el-select></el-form-item>
      <el-form-item v-if="action === 'extend'" label="增加天数"><el-input-number v-model="days" :min="1" :precision="0" class="form-number" /></el-form-item>
      <template v-else>
        <el-form-item label="有效期"><el-select v-model="validityType" class="form-select w-full"><el-option label="指定到期时间" value="duration" /><el-option label="永久" value="permanent" /></el-select></el-form-item>
        <el-form-item v-if="validityType === 'duration'" :label="`到期时间（${businessTimezone}）`"><el-date-picker v-model="expiresAt" type="datetime" value-format="YYYY-MM-DD HH:mm:ss" class="form-date" /></el-form-item>
      </template>
      <button type="submit" class="btn-ember px-4 py-2.5 disabled:opacity-50" :disabled="saving">{{ saving ? '保存中…' : '保存权益' }}</button>
    </el-form>
  </section>
</template>
