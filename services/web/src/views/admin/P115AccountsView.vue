<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Key, Plus } from '@element-plus/icons-vue'

import {
  createP115Account,
  getP115Accounts,
  replaceP115AccountCookie,
  setP115AccountEnabled,
  updateP115AccountSourceLocation,
  validateP115Account,
} from '@/api/admin'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import type {
  CreateP115AccountRequest,
  P115Account,
  P115AccountRole,
  P115AccountStatus,
} from '@/types/api'
import { formatDateTime } from '@/utils/date'

type AccountAction = 'validate' | 'enable'

interface P115AccountForm {
  role: P115AccountRole
  alias: string
  cookie: string
  appType: string
  userAgent: string
  embyPathPrefix: string
  sourceRootId: string
  targetParentId: string
}

interface P115SourceLocationForm {
  embyPathPrefix: string
  sourceRootId: string
}

const accounts = ref<P115Account[]>([])
const loading = ref(false)
const loadFailed = ref(false)
const actionLoading = ref<Record<string, boolean>>({})
let loadRequestVersion = 0

const createDialogVisible = ref(false)
const createSubmitting = ref(false)
const createForm = ref<P115AccountForm>(newCreateForm())

const replaceDialogVisible = ref(false)
const replaceAccount = ref<P115Account | null>(null)
const replacementCookie = ref('')
const replaceSubmitting = ref(false)

const sourceLocationDialogVisible = ref(false)
const sourceLocationAccount = ref<P115Account | null>(null)
const sourceLocationForm = ref<P115SourceLocationForm>({ embyPathPrefix: '', sourceRootId: '' })
const sourceLocationSubmitting = ref(false)

const statusMeta: Record<P115AccountStatus, { label: string; className: string }> = {
  pending: { label: '待验证', className: 'border-amber-100 bg-amber-50 text-amber-700' },
  active: { label: '有效', className: 'border-emerald-100 bg-emerald-50 text-emerald-700' },
  expired: { label: '已失效', className: 'border-red-100 bg-red-50 text-red-700' },
  error: { label: '验证异常', className: 'border-red-100 bg-red-50 text-red-700' },
  cooling_down: { label: '冷却中', className: 'border-sky-100 bg-sky-50 text-sky-700' },
}

const createReady = computed(() => {
  const form = createForm.value
  return Boolean(
    form.alias.trim()
    && form.cookie.trim()
    && form.appType.trim()
    && form.userAgent.trim()
    && (form.role === 'source'
      ? form.embyPathPrefix.trim() && form.sourceRootId.trim()
      : form.targetParentId.trim()),
  )
})

const replaceReady = computed(() => Boolean(replaceAccount.value && replacementCookie.value.trim()))
const sourceLocationReady = computed(() => Boolean(
  sourceLocationAccount.value
  && sourceLocationForm.value.embyPathPrefix.trim()
  && sourceLocationForm.value.sourceRootId.trim(),
))

/** 返回新的空白表单，避免 Cookie 在弹窗之间残留。 */
function newCreateForm(): P115AccountForm {
  return {
    role: 'source',
    alias: '',
    cookie: '',
    appType: 'web',
    userAgent: '',
    embyPathPrefix: '',
    sourceRootId: '',
    targetParentId: '',
  }
}

/** 拉取安全摘要；后端响应没有任何 Cookie 字段。 */
async function loadAccounts(): Promise<void> {
  // 多账号操作可能并发刷新；只允许最后发出的请求覆盖页面状态。
  const requestVersion = ++loadRequestVersion
  loading.value = true
  loadFailed.value = false
  try {
    const response = await getP115Accounts()
    if (requestVersion === loadRequestVersion) {
      accounts.value = response.data
    }
  } catch {
    if (requestVersion === loadRequestVersion) {
      loadFailed.value = true
    }
  } finally {
    if (requestVersion === loadRequestVersion) {
      loading.value = false
    }
  }
}

function openCreateDialog(): void {
  createForm.value = newCreateForm()
  createDialogVisible.value = true
}

/** 关闭创建弹窗时立即丢弃 Cookie 和其他未提交内容。 */
function closeCreateDialog(): void {
  createDialogVisible.value = false
  createForm.value = newCreateForm()
}

/** 创建账号后重新读取服务端摘要，不在前端缓存写入请求中的 Cookie。 */
async function submitCreate(): Promise<void> {
  if (!createReady.value || createSubmitting.value) return

  const form = createForm.value
  const payload: CreateP115AccountRequest = {
    role: form.role,
    alias: form.alias.trim(),
    cookie: form.cookie.trim(),
    appType: form.appType.trim(),
    userAgent: form.userAgent.trim(),
  }
  if (form.role === 'playback') {
    payload.targetParentId = form.targetParentId.trim()
  } else {
    payload.embyPathPrefix = form.embyPathPrefix.trim()
    payload.sourceRootId = form.sourceRootId.trim()
  }

  createSubmitting.value = true
  try {
    await createP115Account(payload)
    closeCreateDialog()
    ElMessage.success('115 账号已添加')
    await loadAccounts()
  } catch {
    // request 拦截器已展示后端的参数或权限错误，表单保持打开便于修正。
  } finally {
    createSubmitting.value = false
  }
}

function actionKey(accountID: string, action: AccountAction): string {
  return `${accountID}:${action}`
}

function isActionLoading(accountID: string, action: AccountAction): boolean {
  return actionLoading.value[actionKey(accountID, action)] === true
}

function isAccountBusy(accountID: string): boolean {
  return isActionLoading(accountID, 'validate') || isActionLoading(accountID, 'enable')
}

function setActionLoading(accountID: string, action: AccountAction, value: boolean): void {
  actionLoading.value = {
    ...actionLoading.value,
    [actionKey(accountID, action)]: value,
  }
}

/** 验证可能以 502 失败但仍持久化 error 状态，因此失败后也重新拉取摘要。 */
async function handleValidate(account: P115Account): Promise<void> {
  if (isAccountBusy(account.id)) return

  setActionLoading(account.id, 'validate', true)
  try {
    const result = await validateP115Account(account.id)
    if (result.valid) {
      ElMessage.success('115 账号验证通过')
    } else {
      ElMessage.warning('Cookie 已失效，请替换后重新验证')
    }
    await loadAccounts()
  } catch {
    await loadAccounts()
  } finally {
    setActionLoading(account.id, 'validate', false)
  }
}

/** 只有 active 账号可以启用；停用不受当前验证状态限制。 */
async function handleEnabled(account: P115Account): Promise<void> {
  const enabled = !account.enabled
  if (enabled && account.status !== 'active') return
  if (enabled && account.role === 'source' && !hasSourceLocation(account)) return
  if (isAccountBusy(account.id)) return

  setActionLoading(account.id, 'enable', true)
  try {
    await setP115AccountEnabled(account.id, enabled)
    ElMessage.success(enabled ? '115 账号已启用' : '115 账号已停用')
    await loadAccounts()
  } catch {
    // 409 等业务冲突由 request 拦截器使用后端原始原因提示。
  } finally {
    setActionLoading(account.id, 'enable', false)
  }
}

/** source 账号只有两个位置字段都存在时才具备运行条件。 */
function hasSourceLocation(account: P115Account): boolean {
  return Boolean(account.embyPathPrefix?.trim() && account.sourceRootId?.trim())
}

/** 打开 source 位置弹窗，只回填后端允许公开的路径摘要。 */
function openSourceLocationDialog(account: P115Account): void {
  if (account.role !== 'source' || isAccountBusy(account.id)) return
  sourceLocationAccount.value = account
  sourceLocationForm.value = {
    embyPathPrefix: account.embyPathPrefix || '',
    sourceRootId: account.sourceRootId || '',
  }
  sourceLocationDialogVisible.value = true
}

/** 关闭源目录弹窗时清空账号引用和未提交路径。 */
function closeSourceLocationDialog(): void {
  sourceLocationDialogVisible.value = false
  sourceLocationAccount.value = null
  sourceLocationForm.value = { embyPathPrefix: '', sourceRootId: '' }
}

/** 保存 source 账号的一对一运行路径配置并刷新安全摘要。 */
async function submitSourceLocation(): Promise<void> {
  if (!sourceLocationReady.value || !sourceLocationAccount.value || sourceLocationSubmitting.value) return
  sourceLocationSubmitting.value = true
  try {
    await updateP115AccountSourceLocation(sourceLocationAccount.value.id, {
      embyPathPrefix: sourceLocationForm.value.embyPathPrefix.trim(),
      sourceRootId: sourceLocationForm.value.sourceRootId.trim(),
    })
    closeSourceLocationDialog()
    ElMessage.success('源目录配置已保存')
    await loadAccounts()
  } catch {
    // request 拦截器已展示后端的路径或角色错误，保留输入便于修正。
  } finally {
    sourceLocationSubmitting.value = false
  }
}

function openReplaceDialog(account: P115Account): void {
  if (isAccountBusy(account.id)) return
  replaceAccount.value = account
  replacementCookie.value = ''
  replaceDialogVisible.value = true
}

/** 关闭替换弹窗时销毁输入值，旧 Cookie 从不进入页面状态。 */
function closeReplaceDialog(): void {
  replaceDialogVisible.value = false
  replaceAccount.value = null
  replacementCookie.value = ''
}

/** 替换成功后账号会回到 pending + disabled，必须由管理员重新验证。 */
async function submitReplacement(): Promise<void> {
  if (!replaceReady.value || !replaceAccount.value || replaceSubmitting.value) return

  replaceSubmitting.value = true
  try {
    await replaceP115AccountCookie(replaceAccount.value.id, replacementCookie.value.trim())
    closeReplaceDialog()
    ElMessage.success('Cookie 已替换，请重新验证账号')
    await loadAccounts()
  } catch {
    // request 拦截器已展示错误；保留本次新输入，便于管理员修正后重试。
  } finally {
    replaceSubmitting.value = false
  }
}

function roleLabel(role: P115AccountRole): string {
  return role === 'source' ? '源账号' : '播放账号'
}

onMounted(() => {
  void loadAccounts()
})
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="115 账号"
      description="Cookie 仅在创建或替换时写入，保存后不会回显。"
    >
      <template #titleSuffix>
        <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">
          {{ accounts.length }} 个账号
        </span>
      </template>

      <template #actions>
        <button
          type="button"
          class="btn-ember inline-flex cursor-pointer items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold shadow-sm"
          @click="openCreateDialog"
        >
          <el-icon><Plus /></el-icon>
          添加账号
        </button>
      </template>
    </EmberPageHeaderCard>

    <section v-loading="loading" class="min-h-40">
      <EmberEmptyStateCard
        v-if="loadFailed"
        title="115 账号加载失败"
        description="请确认登录状态和 API 服务后重试。"
        tone="danger"
        :icon="Key"
      >
        <template #actions>
          <button
            type="button"
            class="cursor-pointer rounded-xl border border-red-200 bg-white px-4 py-2 text-sm font-medium text-red-600"
            @click="loadAccounts"
          >
            重新加载
          </button>
        </template>
      </EmberEmptyStateCard>

      <EmberEmptyStateCard
        v-else-if="!loading && accounts.length === 0"
        title="还没有 115 账号"
        description="添加源账号或播放账号后，再进行验证和启用。"
        :icon="Key"
      >
        <template #actions>
          <button
            type="button"
            class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700"
            @click="openCreateDialog"
          >
            添加账号
          </button>
        </template>
      </EmberEmptyStateCard>

      <div v-else class="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <article
          v-for="account in accounts"
          :key="account.id"
          class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm"
        >
          <header class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <h2 class="truncate text-lg font-semibold text-gray-900">{{ account.alias }}</h2>
              <p class="mt-1 break-all font-mono text-xs text-gray-400">{{ account.id }}</p>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <span class="rounded-full border border-gray-200 bg-gray-50 px-2.5 py-1 text-xs font-medium text-gray-600">
                {{ roleLabel(account.role) }}
              </span>
              <span
                class="rounded-full border px-2.5 py-1 text-xs font-medium"
                :class="statusMeta[account.status].className"
              >
                {{ statusMeta[account.status].label }}
              </span>
              <span
                class="rounded-full border px-2.5 py-1 text-xs font-medium"
                :class="account.enabled
                  ? 'border-emerald-100 bg-emerald-50 text-emerald-700'
                  : 'border-gray-200 bg-gray-50 text-gray-500'"
              >
                {{ account.enabled ? '已启用' : '已停用' }}
              </span>
            </div>
          </header>

          <dl class="grid grid-cols-1 gap-x-6 gap-y-4 px-5 py-5 text-sm sm:grid-cols-2">
            <div>
              <dt class="text-xs text-gray-400">115 UID</dt>
              <dd class="mt-1 break-all font-medium text-gray-700">{{ account.providerUserId || '未识别' }}</dd>
            </div>
            <div>
              <dt class="text-xs text-gray-400">最后验证</dt>
              <dd class="mt-1 font-medium text-gray-700">{{ formatDateTime(account.lastValidatedAt) }}</dd>
            </div>
            <div>
              <dt class="text-xs text-gray-400">客户端类型</dt>
              <dd class="mt-1 break-all font-medium text-gray-700">{{ account.appType }}</dd>
            </div>
            <div v-if="account.role === 'playback'">
              <dt class="text-xs text-gray-400">目标目录 ID</dt>
              <dd class="mt-1 break-all font-medium text-gray-700">{{ account.targetParentId || '-' }}</dd>
            </div>
            <div v-if="account.role === 'source'" class="sm:col-span-2">
              <dt class="text-xs text-gray-400">Emby 挂载目录</dt>
              <dd class="mt-1 break-all font-medium text-gray-700">{{ account.embyPathPrefix || '未配置' }}</dd>
            </div>
            <div v-if="account.role === 'source'">
              <dt class="text-xs text-gray-400">115 源目录 ID</dt>
              <dd class="mt-1 break-all font-medium text-gray-700">{{ account.sourceRootId || '未配置' }}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-xs text-gray-400">User-Agent</dt>
              <dd class="mt-1 break-all font-medium text-gray-700">{{ account.userAgent }}</dd>
            </div>
          </dl>

          <div
            v-if="account.lastErrorMessage"
            class="mx-5 mb-5 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-red-700"
          >
            <p class="font-medium">{{ account.lastErrorMessage }}</p>
            <p v-if="account.lastErrorCode" class="mt-1 font-mono text-xs text-red-500">
              {{ account.lastErrorCode }}
            </p>
          </div>

          <footer class="flex flex-wrap justify-end gap-2 border-t border-gray-100 bg-gray-50/50 px-5 py-4">
            <button
              v-if="account.role === 'source'"
              type="button"
              class="cursor-pointer rounded-xl border border-gray-200 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="isAccountBusy(account.id)"
              @click="openSourceLocationDialog(account)"
            >
              配置源目录
            </button>
            <button
              type="button"
              class="cursor-pointer rounded-xl border border-gray-200 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="isAccountBusy(account.id)"
              @click="openReplaceDialog(account)"
            >
              替换 Cookie
            </button>
            <button
              type="button"
              class="cursor-pointer rounded-xl border border-gray-200 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="isAccountBusy(account.id)"
              @click="handleValidate(account)"
            >
              {{ isActionLoading(account.id, 'validate') ? '验证中' : '验证' }}
            </button>
            <button
              type="button"
              class="cursor-pointer rounded-xl px-3.5 py-2 text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50"
              :class="account.enabled
                ? 'border border-gray-200 bg-white text-gray-700 hover:bg-gray-50'
                : 'btn-ember'"
              :disabled="isAccountBusy(account.id)
                || (!account.enabled && account.status !== 'active')
                || (!account.enabled && account.role === 'source' && !hasSourceLocation(account))"
              @click="handleEnabled(account)"
            >
              {{ isActionLoading(account.id, 'enable') ? '处理中' : (account.enabled ? '停用' : '启用') }}
            </button>
          </footer>
        </article>
      </div>
    </section>

    <EmberFormDialog
      :model-value="createDialogVisible"
      title="添加 115 账号"
      width="680px"
      @update:model-value="$event ? (createDialogVisible = true) : closeCreateDialog()"
    >
      <div class="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <label class="space-y-2 text-sm font-medium text-gray-700">
          <span>账号角色</span>
          <el-select v-model="createForm.role" class="w-full form-select">
            <el-option label="源账号" value="source" />
            <el-option label="播放账号" value="playback" />
          </el-select>
        </label>

        <label class="space-y-2 text-sm font-medium text-gray-700">
          <span>账号别名</span>
          <el-input
            v-model="createForm.alias"
            :placeholder="createForm.role === 'source' ? '例如：源账号' : '例如：播放小号'"
            maxlength="100"
            class="input-ember"
          />
        </label>

        <label class="space-y-2 text-sm font-medium text-gray-700">
          <span>客户端类型</span>
          <el-input v-model="createForm.appType" placeholder="例如：web" maxlength="32" class="input-ember" />
        </label>

        <label
          v-if="createForm.role === 'playback'"
          class="space-y-2 text-sm font-medium text-gray-700"
        >
          <span>目标目录 ID</span>
          <el-input
            v-model="createForm.targetParentId"
            placeholder="请输入播放小号目标目录 ID"
            maxlength="64"
            class="input-ember"
          />
        </label>

        <template v-if="createForm.role === 'source'">
          <label class="space-y-2 text-sm font-medium text-gray-700">
            <span>115 源目录 ID</span>
            <el-input
              v-model="createForm.sourceRootId"
              placeholder="例如：0"
              maxlength="64"
              class="input-ember"
            />
          </label>

          <label class="space-y-2 text-sm font-medium text-gray-700 sm:col-span-2">
            <span>Emby 挂载目录</span>
            <el-input
              v-model="createForm.embyPathPrefix"
              placeholder="例如：/mnt/cloudNAS/115lifetime"
              maxlength="4096"
              class="input-ember"
            />
          </label>
        </template>

        <label class="space-y-2 text-sm font-medium text-gray-700 sm:col-span-2">
          <span>User-Agent</span>
          <el-input
            v-model="createForm.userAgent"
            placeholder="请输入固定的 User-Agent"
            maxlength="512"
            class="input-ember"
          />
        </label>

        <label class="space-y-2 text-sm font-medium text-gray-700 sm:col-span-2">
          <span>Cookie</span>
          <el-input
            v-model="createForm.cookie"
            type="password"
            placeholder="粘贴完整 Cookie"
            autocomplete="off"
            class="input-ember"
          />
        </label>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700"
            @click="closeCreateDialog"
          >
            取消
          </button>
          <button
            type="button"
            class="btn-ember cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!createReady || createSubmitting"
            @click="submitCreate"
          >
            {{ createSubmitting ? '保存中' : '保存账号' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      :model-value="sourceLocationDialogVisible"
      title="配置源目录"
      width="520px"
      @update:model-value="$event ? (sourceLocationDialogVisible = true) : closeSourceLocationDialog()"
    >
      <div class="space-y-4">
        <label class="block space-y-2 text-sm font-medium text-gray-700">
          <span>Emby 挂载目录</span>
          <el-input
            v-model="sourceLocationForm.embyPathPrefix"
            placeholder="例如：/mnt/cloudNAS/115lifetime"
            maxlength="4096"
            class="input-ember"
          />
        </label>
        <label class="block space-y-2 text-sm font-medium text-gray-700">
          <span>115 源目录 ID</span>
          <el-input
            v-model="sourceLocationForm.sourceRootId"
            placeholder="例如：0"
            maxlength="64"
            class="input-ember"
          />
        </label>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700"
            @click="closeSourceLocationDialog"
          >
            取消
          </button>
          <button
            type="button"
            class="btn-ember cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!sourceLocationReady || sourceLocationSubmitting"
            @click="submitSourceLocation"
          >
            {{ sourceLocationSubmitting ? '保存中' : '保存源目录' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      :model-value="replaceDialogVisible"
      title="替换 Cookie"
      width="520px"
      @update:model-value="$event ? (replaceDialogVisible = true) : closeReplaceDialog()"
    >
      <div class="space-y-4">
        <div class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
          保存后账号将停用并回到待验证状态。
        </div>
        <label class="block space-y-2 text-sm font-medium text-gray-700">
          <span>新 Cookie</span>
          <el-input
            v-model="replacementCookie"
            type="password"
            placeholder="粘贴新的完整 Cookie"
            autocomplete="off"
            class="input-ember"
          />
        </label>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700"
            @click="closeReplaceDialog"
          >
            取消
          </button>
          <button
            type="button"
            class="btn-ember cursor-pointer rounded-xl px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!replaceReady || replaceSubmitting"
            @click="submitReplacement"
          >
            {{ replaceSubmitting ? '替换中' : '确认替换' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>
  </div>
</template>
