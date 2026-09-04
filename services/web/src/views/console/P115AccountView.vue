<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Check, Key, Loading, Setting } from '@element-plus/icons-vue'

import {
  createPersonalP115Account,
  getPersonalP115Account,
  getPersonalP115Usage,
  replacePersonalP115Cookie,
  revokePersonalP115Account,
  setPersonalP115Enabled,
  updatePersonalP115Concurrency,
  updatePersonalP115Directory,
  validatePersonalP115Account,
} from '@/api/console'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import type { P115AccountStatus, PersonalP115Account, PersonalP115Usage } from '@/types/api'

type AccountAction = 'create' | 'replace' | 'validate' | 'directory' | 'concurrency' | 'enabled' | 'revoke'

const account = ref<PersonalP115Account | null>(null)
const usage = ref<PersonalP115Usage | null>(null)
const loading = ref(false)
const loadFailed = ref(false)
const activeAction = ref<AccountAction | null>(null)
const createCookie = ref('')
const replacementCookie = ref('')
const showReplacement = ref(false)
const targetParentPath = ref('')
const maxConcurrentStreams = ref(1)

const statusPresentation: Record<P115AccountStatus, { label: string; className: string }> = {
  pending: { label: '待验证', className: 'border-amber-100 bg-amber-50 text-amber-700' },
  active: { label: '有效', className: 'border-emerald-100 bg-emerald-50 text-emerald-700' },
  expired: { label: '已失效', className: 'border-red-100 bg-red-50 text-red-700' },
  error: { label: '验证异常', className: 'border-red-100 bg-red-50 text-red-700' },
  cooling_down: { label: '冷却中', className: 'border-sky-100 bg-sky-50 text-sky-700' },
  revoked: { label: '已撤销', className: 'border-gray-200 bg-gray-50 text-gray-600' },
}

const isBusy = computed(() => activeAction.value !== null)
const isValidated = computed(() => account.value?.status === 'active')
const isConfigured = computed(() => Boolean(
  account.value?.targetParentPath?.trim()
  && account.value.maxConcurrentStreams
  && account.value.maxConcurrentStreams > 0,
))
const concurrencyUpperBound = computed(() => {
  const planLimit = account.value?.simultaneousStreamLimit ?? 0
  return planLimit > 0 ? planLimit : 100
})
const canEnable = computed(() => isValidated.value
  && isConfigured.value
  && (account.value?.maxConcurrentStreams ?? 0) <= concurrencyUpperBound.value)
const clientLabel = computed(() => {
  const appType = account.value?.appType?.trim()
  return !appType || appType === 'unknown' ? '未识别' : appType
})
const playbackModeLabel = computed(() => account.value?.p115PlaybackMode === 'system' ? '系统共享账号' : '个人账号')

function isHTTPStatus(error: unknown, status: number): boolean {
  return typeof error === 'object'
    && error !== null
    && 'response' in error
    && (error as { response?: { status?: number } }).response?.status === status
}

/** 用完整查询摘要重置页面表单，避免把局部写响应误当成套餐事实。 */
function applyAccount(next: PersonalP115Account | null): void {
  account.value = next
  targetParentPath.value = next?.targetParentPath ?? ''
  maxConcurrentStreams.value = next?.maxConcurrentStreams ?? 1
}

/** 查询当前用户唯一的非 revoked 账号；404 是正常未绑定状态。 */
async function loadAccount(): Promise<void> {
	loading.value = true
	loadFailed.value = false
	const usageRequest = getPersonalP115Usage().catch(() => null)
	try {
		applyAccount(await getPersonalP115Account())
  } catch (error) {
    if (isHTTPStatus(error, 404)) {
      applyAccount(null)
    } else {
			loadFailed.value = true
		}
	} finally {
		usage.value = await usageRequest
		loading.value = false
	}
}

/** 创建请求只发送 Cookie，成功后立即销毁输入并重读完整摘要。 */
async function handleCreate(): Promise<void> {
  const cookie = createCookie.value.trim()
  if (!cookie || isBusy.value) return
  activeAction.value = 'create'
  try {
    await createPersonalP115Account(cookie)
    createCookie.value = ''
    ElMessage.success('115 账号已绑定，请验证 Cookie')
    await loadAccount()
  } catch {
    // request 拦截器已展示绑定失败原因，保留输入便于修正。
  } finally {
    activeAction.value = null
  }
}

/** Cookie 替换后销毁输入；后端会重置验证状态和旧目录。 */
async function handleReplacement(): Promise<void> {
  const cookie = replacementCookie.value.trim()
  if (!cookie || isBusy.value) return
  activeAction.value = 'replace'
  try {
    await replacePersonalP115Cookie(cookie)
    replacementCookie.value = ''
    showReplacement.value = false
    ElMessage.success('Cookie 已替换，请重新验证')
    await loadAccount()
  } catch {
    // request 拦截器已展示替换失败原因，保留本次输入。
  } finally {
    activeAction.value = null
  }
}

/** 验证失败也可能持久化账号状态，因此无论结果如何都重新查询。 */
async function handleValidate(): Promise<void> {
  if (!account.value || isBusy.value) return
  activeAction.value = 'validate'
  try {
    const result = await validatePersonalP115Account()
    if (result.valid) {
      ElMessage.success('Cookie 验证通过')
    } else {
      ElMessage.warning('Cookie 已失效，请替换后重试')
    }
  } catch {
    // Provider 错误状态可能已落库，统一在 finally 中重读。
  } finally {
    await loadAccount()
    activeAction.value = null
  }
}

/** 保存已存在的目录路径；内部目录 ID 只由后端解析和持久化。 */
async function handleDirectory(): Promise<void> {
  const path = targetParentPath.value.trim()
  if (!path || !isValidated.value || isBusy.value) return
  activeAction.value = 'directory'
  try {
    await updatePersonalP115Directory(path)
    ElMessage.success('目标目录已保存')
    await loadAccount()
  } catch {
    // request 拦截器已展示目录解析失败原因，保留路径便于修正。
  } finally {
    activeAction.value = null
  }
}

/** 保存配置值；页面展示的 effective 值不会反向覆盖该输入。 */
async function handleConcurrency(): Promise<void> {
  const value = maxConcurrentStreams.value
  if (!Number.isInteger(value) || value < 1 || value > concurrencyUpperBound.value || !isValidated.value || isBusy.value) return
  activeAction.value = 'concurrency'
  try {
    await updatePersonalP115Concurrency(value)
    ElMessage.success('最大播放路数已保存')
    await loadAccount()
  } catch {
    // request 拦截器已展示套餐或配置冲突，保留配置值。
  } finally {
    activeAction.value = null
  }
}

/** 启用时依赖后端再次原子校验完整账号和当前套餐。 */
async function handleEnabled(): Promise<void> {
  if (!account.value || isBusy.value) return
  const enabled = !account.value.enabled
  if (enabled && !canEnable.value) return
  activeAction.value = 'enabled'
  try {
    await setPersonalP115Enabled(enabled)
    ElMessage.success(enabled ? '115 账号已启用' : '115 账号已停用')
    await loadAccount()
  } catch {
    // 后端是启用前完整性和套餐校验的最终事实源。
  } finally {
    activeAction.value = null
  }
}

/** 解绑是不可逆凭证擦除，确认后由后端保留无凭证 tombstone。 */
async function handleRevoke(): Promise<void> {
  if (!account.value || isBusy.value) return
  try {
    await ElMessageBox.confirm(
      '解绑后 Cookie 和账号配置会被永久擦除，确定继续吗？',
      '解绑 115 账号',
      { confirmButtonText: '解绑', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' },
    )
  } catch {
    return
  }

  activeAction.value = 'revoke'
  try {
    await revokePersonalP115Account()
    replacementCookie.value = ''
    showReplacement.value = false
    ElMessage.success('115 账号已解绑')
    await loadAccount()
  } catch {
    // request 拦截器已展示解绑失败原因，当前账号保持不变。
  } finally {
    activeAction.value = null
  }
}

onMounted(() => {
  void loadAccount()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="115 网盘"
      description="Cookie 只在提交时写入，保存后不会回显。"
    >
      <template v-if="account" #titleSuffix>
        <span
          class="rounded-full border px-2.5 py-1 text-xs font-medium"
          :class="statusPresentation[account.status].className"
        >
          {{ statusPresentation[account.status].label }}
        </span>
      </template>
    </EmberPageHeaderCard>

    <section v-loading="loading" class="min-h-40">
      <EmberEmptyStateCard
        v-if="loadFailed"
        title="115 账号加载失败"
        description="请稍后重试。"
        tone="danger"
        :icon="Key"
      >
        <template #actions>
          <button
            type="button"
            class="cursor-pointer rounded-xl border border-red-200 bg-white px-4 py-2.5 text-sm font-medium text-red-600"
            @click="loadAccount"
          >
            重新加载
          </button>
        </template>
      </EmberEmptyStateCard>

      <div v-else-if="!loading && !account" class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="max-w-2xl">
          <h2 class="text-lg font-semibold text-gray-900">绑定个人账号</h2>
          <label class="mt-5 block space-y-2 text-sm font-medium text-gray-700">
            <span>Cookie</span>
            <el-input
              v-model="createCookie"
              type="password"
              show-password
              autocomplete="off"
              placeholder="粘贴完整 Cookie"
              class="input-ember"
            />
          </label>
          <button
            type="button"
            class="btn-ember mt-4 cursor-pointer rounded-xl px-5 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!createCookie.trim() || isBusy"
            @click="handleCreate"
          >
            {{ activeAction === 'create' ? '绑定中' : '绑定账号' }}
          </button>
        </div>
        <div v-if="usage?.p115PlaybackMode === 'system'" class="mt-6 border-t border-gray-100 pt-5 text-sm text-gray-600">
          当前套餐使用系统共享账号；绑定个人账号不会改变当前播放路由。
        </div>
      </div>

      <div v-else-if="account" class="space-y-6">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
          <div
            v-for="step in [
              { label: '绑定 Cookie', done: true },
              { label: '验证 Cookie', done: isValidated },
              { label: '配置账号', done: isConfigured },
              { label: '启用账号', done: account.enabled },
            ]"
            :key="step.label"
            class="flex items-center gap-3 rounded-2xl border bg-white px-4 py-3 shadow-sm"
            :class="step.done ? 'border-emerald-100' : 'border-gray-100'"
          >
            <span
              class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full"
              :class="step.done ? 'bg-emerald-50 text-emerald-600' : 'bg-gray-100 text-gray-400'"
            >
              <el-icon><Check v-if="step.done" /><Loading v-else /></el-icon>
            </span>
            <span class="text-sm font-medium" :class="step.done ? 'text-gray-900' : 'text-gray-500'">{{ step.label }}</span>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.3fr)_minmax(18rem,0.7fr)]">
          <article class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900">账号状态</h2>
                <p class="mt-1 text-sm text-gray-500">{{ playbackModeLabel }}</p>
              </div>
              <span
                class="w-fit rounded-full border px-2.5 py-1 text-xs font-medium"
                :class="account.enabled
                  ? 'border-emerald-100 bg-emerald-50 text-emerald-700'
                  : 'border-gray-200 bg-gray-50 text-gray-600'"
              >
                {{ account.enabled ? '已启用' : '已停用' }}
              </span>
            </div>

            <dl class="mt-5 grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
              <div>
                <dt class="text-xs text-gray-500">115 UID</dt>
                <dd class="mt-1 font-medium text-gray-900">{{ account.providerUserId || '待验证' }}</dd>
              </div>
              <div>
                <dt class="text-xs text-gray-500">客户端</dt>
                <dd class="mt-1 font-medium text-gray-900">{{ clientLabel }}</dd>
              </div>
              <div>
                <dt class="text-xs text-gray-500">目标目录</dt>
                <dd class="mt-1 break-all font-medium text-gray-900">{{ account.targetParentPath || '未配置' }}</dd>
              </div>
              <div>
                <dt class="text-xs text-gray-500">播放路数</dt>
                <dd class="mt-1 font-medium tabular-nums text-gray-900">
                  <template v-if="account.maxConcurrentStreams">
                    配置 {{ account.maxConcurrentStreams }} 路
                    <span v-if="account.effectiveMaxConcurrentStreams !== undefined" class="text-gray-500">
                      · 当前有效 {{ account.effectiveMaxConcurrentStreams }} 路
                    </span>
                  </template>
                  <template v-else>未配置</template>
                </dd>
              </div>
            </dl>

            <div class="mt-6 flex flex-wrap gap-2 border-t border-gray-100 pt-5">
              <button
                type="button"
                class="btn-ember cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="isBusy || account.status === 'revoked'"
                @click="handleValidate"
              >
                {{ activeAction === 'validate' ? '验证中' : '验证 Cookie' }}
              </button>
              <button
                type="button"
                class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="isBusy"
                @click="showReplacement = !showReplacement"
              >
                替换 Cookie
              </button>
              <button
                type="button"
                class="cursor-pointer rounded-xl border border-red-200 bg-red-50 px-4 py-2.5 text-sm font-medium text-red-600 disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="isBusy"
                @click="handleRevoke"
              >
                {{ activeAction === 'revoke' ? '解绑中' : '解绑账号' }}
              </button>
            </div>

            <div v-if="showReplacement" class="mt-4 rounded-2xl border border-amber-100 bg-amber-50/60 p-4">
              <label class="block space-y-2 text-sm font-medium text-gray-700">
                <span>新 Cookie</span>
                <el-input
                  v-model="replacementCookie"
                  type="password"
                  show-password
                  autocomplete="off"
                  placeholder="粘贴新的完整 Cookie"
                  class="input-ember"
                />
              </label>
              <button
                type="button"
                class="btn-ember mt-3 cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="!replacementCookie.trim() || isBusy"
                @click="handleReplacement"
              >
                {{ activeAction === 'replace' ? '替换中' : '确认替换' }}
              </button>
            </div>
          </article>

          <aside class="space-y-5">
            <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
              <div class="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <el-icon class="text-ember"><Setting /></el-icon>
                播放用量
              </div>
              <div v-if="account.usageAvailable" class="mt-4 grid grid-cols-3 gap-3 text-center">
                <div><div class="text-xl font-semibold tabular-nums text-gray-900">{{ account.reservedStreams ?? 0 }}</div><div class="mt-1 text-xs text-gray-500">准备中</div></div>
                <div><div class="text-xl font-semibold tabular-nums text-gray-900">{{ account.activeStreams ?? 0 }}</div><div class="mt-1 text-xs text-gray-500">播放中</div></div>
                <div><div class="text-xl font-semibold tabular-nums text-gray-900">{{ account.occupiedStreams ?? 0 }}</div><div class="mt-1 text-xs text-gray-500">总占用</div></div>
              </div>
              <p v-else class="mt-4 text-sm font-medium text-amber-700">用量不可用</p>
              <div v-if="usage?.usageAvailable" class="mt-4 border-t border-gray-100 pt-4">
                <div class="grid grid-cols-3 gap-3 text-center">
                  <div><div class="text-lg font-semibold tabular-nums text-gray-900">{{ usage.userReservedStreams ?? 0 }}</div><div class="mt-1 text-xs text-gray-500">本人准备中</div></div>
                  <div><div class="text-lg font-semibold tabular-nums text-gray-900">{{ usage.userActiveStreams ?? 0 }}</div><div class="mt-1 text-xs text-gray-500">本人播放中</div></div>
                  <div><div class="text-lg font-semibold tabular-nums text-gray-900">{{ usage.userOccupiedStreams ?? 0 }}</div><div class="mt-1 text-xs text-gray-500">本人总占用</div></div>
                </div>
              </div>
            </div>

            <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
              <h2 class="text-sm font-semibold text-gray-900">套餐限制</h2>
              <dl class="mt-4 space-y-3 text-sm">
                <div class="flex items-center justify-between gap-4"><dt class="text-gray-500">同时播放</dt><dd class="font-medium tabular-nums text-gray-900">{{ account.simultaneousStreamLimit === 0 ? '无有限上限' : `${account.simultaneousStreamLimit ?? '-'} 路` }}</dd></div>
                <div class="flex items-center justify-between gap-4"><dt class="text-gray-500">滚动小时转存</dt><dd class="font-medium tabular-nums text-gray-900">{{ usage?.usageAvailable ? `${usage.transferHourlyUsed ?? 0} / ${usage.transferHourlyLimit}` : '-' }}</dd></div>
                <div class="flex items-center justify-between gap-4"><dt class="text-gray-500">每日转存</dt><dd class="font-medium tabular-nums text-gray-900">{{ usage?.usageAvailable ? `${usage.transferDailyUsed ?? 0} / ${usage.transferDailyLimit}` : '-' }}</dd></div>
                <div v-if="usage?.usageAvailable && (usage.transferPending ?? 0) > 0" class="flex items-center justify-between gap-4"><dt class="text-gray-500">转存处理中</dt><dd class="font-medium tabular-nums text-gray-900">{{ usage.transferPending }}</dd></div>
              </dl>
            </div>
          </aside>
        </div>

        <article class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
          <h2 class="text-lg font-semibold text-gray-900">账号配置</h2>
          <div v-if="isValidated" class="mt-5 grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div>
              <label class="block space-y-2 text-sm font-medium text-gray-700">
                <span>目标目录路径</span>
                <el-input
                  v-model="targetParentPath"
                  placeholder="例如：/Ember/Playback"
                  maxlength="4096"
                  class="input-ember"
                />
              </label>
              <button
                type="button"
                class="btn-ember mt-3 cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="!targetParentPath.trim() || isBusy"
                @click="handleDirectory"
              >
                {{ activeAction === 'directory' ? '保存中' : '保存目录' }}
              </button>
            </div>

            <div>
              <label class="block space-y-2 text-sm font-medium text-gray-700">
                <span>最大播放路数</span>
                <el-input-number
                  v-model="maxConcurrentStreams"
                  :min="1"
                  :max="concurrencyUpperBound"
                  :step="1"
                  :precision="0"
                  class="w-full !w-full form-number"
                />
              </label>
              <button
                type="button"
                class="btn-ember mt-3 cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="maxConcurrentStreams < 1 || maxConcurrentStreams > concurrencyUpperBound || isBusy"
                @click="handleConcurrency"
              >
                {{ activeAction === 'concurrency' ? '保存中' : '保存播放路数' }}
              </button>
            </div>
          </div>
          <p v-else class="mt-4 text-sm text-gray-500">请先验证 Cookie。</p>

          <div class="mt-6 border-t border-gray-100 pt-5">
            <button
              type="button"
              class="cursor-pointer rounded-xl px-5 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
              :class="account.enabled
                ? 'border border-gray-200 bg-white text-gray-700'
                : 'btn-ember'"
              :disabled="isBusy || (!account.enabled && !canEnable)"
              @click="handleEnabled"
            >
              {{ activeAction === 'enabled' ? '处理中' : (account.enabled ? '停用账号' : '启用账号') }}
            </button>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>
