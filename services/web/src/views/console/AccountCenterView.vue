<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ChatDotRound,
  CopyDocument,
  FolderOpened,
  Key,
  Lock,
  Message,
  Monitor,
  UserFilled
} from '@element-plus/icons-vue'
import EmberFormDialog from '@/components/ember/forms/EmberFormDialog.vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import { formatDateTime } from '@/utils/date'
import { formatMediaLibrarySummary } from '@/utils/media-library'
import { useAuthStore } from '@/store/auth'
import { useUserStore } from '@/store/user'
import { getRegistrationMode } from '@/api/auth'
import { bindAdminEmbyAccount, getAdminEmbyUsers, unbindAdminEmbyAccount } from '@/api/admin'
import {
  generateTelegramBindCode,
  getUserMediaLibraries,
  resetUserMediaLibraryPreferences,
  sendEmailChangeCode,
  unbindTelegram,
  updateUserMediaLibraries,
  updatePassword
} from '@/api/console'
import type { AdminEmbyUserOption, TelegramBindCodeResponse, UserInfo, UserMediaLibrarySettings } from '@/types/api'

const authStore = useAuthStore()
const userStore = useUserStore()

const emptyUser: UserInfo = {
  id: '',
  username: '',
  role: 'user',
  email: '',
  embyId: '',
  embyDisabled: false,
  isActive: false,
  createdAt: ''
}

const user = computed(() => userStore.profile ?? emptyUser)
const loading = ref(false)
const emailInput = ref('')
const telegramBindCode = ref<TelegramBindCodeResponse | null>(null)
const generatingBindCode = ref(false)
const unbinding = ref(false)
const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const verifyDialogVisible = ref(false)
const pendingNewEmail = ref('')
const verifyCode = ref('')
const sendingEmailCode = ref(false)
const confirmingEmailChange = ref(false)
const emailCodeCountdown = ref(0)
const allowedEmailDomains = ref<string[]>([])
const emailDomainError = ref('')
let countdownTimer: ReturnType<typeof setInterval> | null = null

const embyBindDialogVisible = ref(false)
const embyUserSearch = ref('')
const embyUserOptions = ref<AdminEmbyUserOption[]>([])
const selectedEmbyId = ref('')
const loadingEmbyUsers = ref(false)
const bindingEmby = ref(false)
const unbindingEmby = ref(false)
const mediaLibrarySettings = ref<UserMediaLibrarySettings | null>(null)
const selectedMediaLibraryIds = ref<string[]>([])
const loadingMediaLibraries = ref(false)
const savingMediaLibraries = ref(false)
const resettingMediaLibraries = ref(false)

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const hasDomainAllowlist = computed(() => allowedEmailDomains.value.length > 0)

const allowedDomainsHint = computed(() => {
  const domains = allowedEmailDomains.value
  if (domains.length === 0) return ''
  if (domains.length <= 3) {
    return `仅支持以下邮箱域名：${domains.join('、')}`
  }
  const head = domains.slice(0, 2).join('、')
  return `仅支持以下邮箱域名：${head} 等 ${domains.length} 个域名`
})

const extractEmailDomain = (email: string): string => {
  const trimmed = email.trim()
  const atIndex = trimmed.lastIndexOf('@')
  if (atIndex <= 0 || atIndex === trimmed.length - 1) return ''
  return trimmed.slice(atIndex + 1).toLowerCase()
}

const isEmailDomainAllowed = (email: string): boolean => {
  if (!hasDomainAllowlist.value) return true
  const domain = extractEmailDomain(email)
  if (!domain) return true
  return allowedEmailDomains.value.includes(domain)
}

const handleEmailBlur = () => {
  if (!hasDomainAllowlist.value) {
    emailDomainError.value = ''
    return
  }
  const email = emailInput.value.trim()
  if (!email) {
    emailDomainError.value = ''
    return
  }
  if (!isEmailDomainAllowed(email)) {
    emailDomainError.value = '该邮箱域名不在允许范围内'
  } else {
    emailDomainError.value = ''
  }
}

const handleEmailInput = () => {
  if (emailDomainError.value) {
    emailDomainError.value = ''
  }
}

const fetchAllowedEmailDomains = async () => {
  try {
    const res = await getRegistrationMode()
    allowedEmailDomains.value = (res.allowedEmailDomains ?? [])
      .map((d) => d.trim().toLowerCase())
      .filter(Boolean)
  } catch {
    // 拉取失败时退化为"无白名单"语义，让用户体验等同于功能未启用；
    // 后端仍会作为唯一可信校验点，最终请求会被拦截。
    allowedEmailDomains.value = []
  }
}

const isTelegramBound = computed(() => !!user.value.telegramId)
const isEmbyLinked = computed(() => !!user.value.embyId)
const hasMediaLibraryPreferences = computed(() => mediaLibrarySettings.value?.customized === true)
const hasMediaLibraryOptions = computed(() => (mediaLibrarySettings.value?.libraries.length ?? 0) > 0)
const mediaLibrarySyncing = computed(() => {
  const status = mediaLibrarySettings.value?.policySyncStatus
  return status === 'pending' || status === 'processing'
})

const embySummary = computed(() => {
  if (isEmbyLinked.value) return '已关联'
  if (authStore.isAdmin) return '可关联'
  return '待开通'
})

/** 判断媒体库偏好保存是否撞上后端同步闸门，用于 409 专门提示。 */
const isConflictError = (error: unknown) => {
  return typeof error === 'object'
    && error !== null
    && 'response' in error
    && (error as { response?: { status?: number } }).response?.status === 409
}

/** 用服务端返回的设置重置本地勾选状态，保证保存前后都以 API 契约为准。 */
const applyMediaLibrarySettings = (settings: UserMediaLibrarySettings) => {
  mediaLibrarySettings.value = settings
  selectedMediaLibraryIds.value = settings.libraries
    .filter(item => item.enabled)
    .map(item => item.id)
}

const showMediaLibrarySaveResult = (settings: UserMediaLibrarySettings, syncedMessage: string) => {
  if (settings.policySyncStatus === 'failed' || settings.policySyncStatus === 'partial_failed') {
    ElMessage.warning('本地已保存，Emby 同步失败，请联系管理员处理')
    return
  }
  if (settings.policySyncStatus === 'pending' || settings.policySyncStatus === 'processing') {
    ElMessage.info('本地已保存，正在等待 Emby 同步')
    return
  }
  ElMessage.success(syncedMessage)
}

/** 读取当前用户的媒体库偏好；接口不可用时保留空态，不阻塞账号中心其他功能。 */
const loadMediaLibraries = async () => {
  loadingMediaLibraries.value = true
  try {
    const res = await getUserMediaLibraries()
    applyMediaLibrarySettings(res.data)
  } catch {
    mediaLibrarySettings.value = null
    selectedMediaLibraryIds.value = []
  } finally {
    loadingMediaLibraries.value = false
  }
}

watch(
  () => user.value.email,
  (value) => {
    emailInput.value = value || ''
  },
  { immediate: true }
)

const syncTelegramBindState = () => {
  if (user.value.telegramId) {
    telegramBindCode.value = null
  }
}

const clearCountdown = () => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

const startCountdown = () => {
  clearCountdown()
  emailCodeCountdown.value = 60
  countdownTimer = setInterval(() => {
    emailCodeCountdown.value -= 1
    if (emailCodeCountdown.value <= 0) {
      emailCodeCountdown.value = 0
      clearCountdown()
    }
  }, 1000)
}

const resetVerifyDialogState = () => {
  verifyDialogVisible.value = false
  verifyCode.value = ''
  pendingNewEmail.value = ''
  clearCountdown()
  emailCodeCountdown.value = 0
}

const handleSaveEmail = async () => {
  const trimmed = emailInput.value.trim()
  if (!EMAIL_PATTERN.test(trimmed)) {
    ElMessage.warning('请输入有效的邮箱地址')
    return
  }
  if (trimmed === (user.value.email || '')) {
    ElMessage.info('邮箱未变更')
    return
  }

  sendingEmailCode.value = true
  try {
    await sendEmailChangeCode(trimmed)
    pendingNewEmail.value = trimmed
    verifyCode.value = ''
    verifyDialogVisible.value = true
    startCountdown()
  } catch {
    // handled
  } finally {
    sendingEmailCode.value = false
  }
}

const handleResendCode = async () => {
  if (emailCodeCountdown.value > 0 || !pendingNewEmail.value) {
    return
  }

  sendingEmailCode.value = true
  try {
    await sendEmailChangeCode(pendingNewEmail.value)
    startCountdown()
  } catch {
    // handled
  } finally {
    sendingEmailCode.value = false
  }
}

const handleConfirmEmailChange = async () => {
  if (verifyCode.value.length !== 6) {
    ElMessage.warning('请输入 6 位验证码')
    return
  }

  confirmingEmailChange.value = true
  try {
    await userStore.updateEmail(pendingNewEmail.value, verifyCode.value)
    ElMessage.success('邮箱更新成功')
    const newEmail = pendingNewEmail.value
    resetVerifyDialogState()
    emailInput.value = newEmail
  } catch {
    // handled，保留弹窗让用户重试 code
  } finally {
    confirmingEmailChange.value = false
  }
}

/** 保存用户在分组模板范围内的媒体库启用集合，提交给后端做完整快照。 */
const handleSaveMediaLibraries = async () => {
  if (selectedMediaLibraryIds.value.length === 0) {
    try {
      await ElMessageBox.confirm(
        '保存后将关闭所有媒体库显示，Emby 客户端中不会保留任何已启用媒体库。确定继续吗？',
        '确认关闭全部媒体库',
        {
          confirmButtonText: '确认关闭',
          cancelButtonText: '取消',
          type: 'warning',
          confirmButtonClass: 'el-button--danger'
        }
      )
    } catch {
      return
    }
  }

  savingMediaLibraries.value = true
  try {
    const res = await updateUserMediaLibraries(selectedMediaLibraryIds.value)
    applyMediaLibrarySettings(res.data)
    showMediaLibrarySaveResult(res.data, '媒体库偏好已保存')
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('媒体库权限正在同步，稍后再保存')
    }
  } finally {
    savingMediaLibraries.value = false
  }
}

/** 清除自定义偏好，让当前用户重新继承所属分组的媒体库模板。 */
const handleResetMediaLibraries = async () => {
  resettingMediaLibraries.value = true
  try {
    const res = await resetUserMediaLibraryPreferences()
    applyMediaLibrarySettings(res.data)
    showMediaLibrarySaveResult(res.data, '已恢复分组默认')
  } catch (error) {
    if (isConflictError(error)) {
      ElMessage.warning('媒体库权限正在同步，稍后再恢复默认')
    }
  } finally {
    resettingMediaLibraries.value = false
  }
}

const handleCancelEmailChange = () => {
  resetVerifyDialogState()
}

const handleUpdatePassword = async () => {
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    ElMessage.warning('两次输入密码不一致')
    return
  }

  try {
    await updatePassword({
      oldPassword: passwordForm.value.oldPassword,
      newPassword: passwordForm.value.newPassword
    })
    ElMessage.success('密码修改成功')
    passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  } catch {
    // handled
  }
}

const handleGenerateBindCode = async () => {
  generatingBindCode.value = true
  try {
    telegramBindCode.value = await generateTelegramBindCode()
  } catch {
    // handled
  } finally {
    generatingBindCode.value = false
  }
}

const handleUnbindTelegram = async () => {
  unbinding.value = true
  try {
    await unbindTelegram()
    ElMessage.success('已解除 Telegram 绑定')
    telegramBindCode.value = null
    if (userStore.profile) {
      userStore.setProfile({
        ...userStore.profile,
        telegramId: undefined
      })
    }
  } catch {
    // handled
  } finally {
    unbinding.value = false
  }
}

const resetEmbyBindState = () => {
  embyUserSearch.value = ''
  selectedEmbyId.value = ''
  embyUserOptions.value = []
}

const loadAdminEmbyUsers = async () => {
  const query = embyUserSearch.value.trim()
  if (query.length < 2) {
    embyUserOptions.value = []
    selectedEmbyId.value = ''
    ElMessage.warning('请输入至少 2 个字符搜索 Emby 用户')
    return
  }

  loadingEmbyUsers.value = true
  try {
    const res = await getAdminEmbyUsers({ query, limit: 20 })
    embyUserOptions.value = res.data
    const current = res.data.find(item => item.boundToCurrent)
    selectedEmbyId.value = current?.embyId ?? ''
  } catch {
    // handled by request interceptor
  } finally {
    loadingEmbyUsers.value = false
  }
}

const handleOpenEmbyBindDialog = () => {
  resetEmbyBindState()
  embyBindDialogVisible.value = true
}

const handleCancelEmbyBind = () => {
  embyBindDialogVisible.value = false
  resetEmbyBindState()
}

const handleSelectEmbyUser = (option: AdminEmbyUserOption) => {
  if (!option.available) {
    return
  }
  selectedEmbyId.value = option.embyId
}

const handleConfirmEmbyBind = async () => {
  const embyId = selectedEmbyId.value.trim()
  if (!embyId) {
    ElMessage.warning('请选择 Emby 用户')
    return
  }

  bindingEmby.value = true
  try {
    await bindAdminEmbyAccount({ embyId })
    await userStore.fetchProfile()
    ElMessage.success('Emby 账号已关联')
    embyBindDialogVisible.value = false
    resetEmbyBindState()
  } catch {
    // handled by request interceptor
  } finally {
    bindingEmby.value = false
  }
}

const handleUnbindEmby = async () => {
  try {
    await ElMessageBox.confirm('解除后，媒体相关功能将无法使用，可重新关联', '解除 Emby 关联', {
      confirmButtonText: '解除',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }

  unbindingEmby.value = true
  try {
    await unbindAdminEmbyAccount()
    await userStore.fetchProfile()
    ElMessage.success('已解除 Emby 关联')
  } catch {
    // handled
  } finally {
    unbindingEmby.value = false
  }
}

const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('复制成功')
  } catch {
    ElMessage.error('复制失败')
  }
}

watch(
  () => user.value.telegramId,
  () => {
    syncTelegramBindState()
  },
  { immediate: true }
)

watch(verifyDialogVisible, (visible) => {
  if (!visible) {
    verifyCode.value = ''
    pendingNewEmail.value = ''
    clearCountdown()
    emailCodeCountdown.value = 0
  }
})

onBeforeUnmount(() => {
  clearCountdown()
})

onMounted(() => {
  fetchAllowedEmailDomains()
  loadMediaLibraries()
})
</script>

<template>
  <div class="space-y-6" v-loading="loading">
    <div class="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(22rem,0.85fr)]">
      <section class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-ember/10 text-ember">
            <el-icon><UserFilled /></el-icon>
          </div>
          <div>
            <h2 class="text-lg font-semibold text-gray-900">基本资料</h2>
          </div>
        </div>

        <div class="mt-6 grid gap-5 md:grid-cols-2">
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">用户名</p>
            <div class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700">
              {{ user.username || '-' }}
            </div>
          </div>
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">有效期</p>
            <div class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700">
              {{ user.expiresAt ? formatDateTime(user.expiresAt, 'short') : '永久有效' }}
            </div>
          </div>
          <div class="space-y-2 md:col-span-2">
            <label for="account-email" class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">联系邮箱</label>
            <div class="flex flex-col gap-3 sm:flex-row">
              <el-input
                id="account-email"
                v-model="emailInput"
                type="email"
                placeholder="name@example.com"
                :prefix-icon="Message"
                class="input-ember sm:flex-1"
                @blur="handleEmailBlur"
                @input="handleEmailInput"
              />
              <button
                class="btn-ember rounded-2xl px-5 py-3 text-sm font-semibold cursor-pointer disabled:opacity-60"
                :disabled="sendingEmailCode"
                @click="handleSaveEmail"
              >
                {{ sendingEmailCode ? '发送中...' : '保存邮箱' }}
              </button>
            </div>
            <p v-if="emailDomainError" class="text-xs text-red-600 leading-5">
              {{ emailDomainError }}
            </p>
            <p v-else-if="hasDomainAllowlist" class="text-xs text-gray-500 leading-5">
              {{ allowedDomainsHint }}
            </p>
            <p class="text-xs text-gray-500">用于找回密码和接收系统通知。</p>
          </div>
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">创建时间</p>
            <div class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700">
              {{ formatDateTime(user.createdAt, 'short') }}
            </div>
          </div>
        </div>
      </section>

      <section class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-gray-100 text-gray-700">
            <el-icon><Key /></el-icon>
          </div>
          <div>
            <h2 class="text-lg font-semibold text-gray-900">安全设置</h2>
          </div>
        </div>

        <div class="mt-6 space-y-4">
          <div class="space-y-1">
            <label for="current-password" class="ml-1 text-xs font-semibold uppercase tracking-wider text-gray-400">当前密码</label>
            <el-input
              id="current-password"
              v-model="passwordForm.oldPassword"
              type="password"
              show-password
              placeholder="请输入当前密码"
              :prefix-icon="Lock"
              class="input-ember"
            />
          </div>
          <div class="space-y-1">
            <label for="new-password" class="ml-1 text-xs font-semibold uppercase tracking-wider text-gray-400">新密码</label>
            <el-input
              id="new-password"
              v-model="passwordForm.newPassword"
              type="password"
              show-password
              placeholder="请输入新密码"
              :prefix-icon="Lock"
              class="input-ember"
            />
          </div>
          <div class="space-y-1">
            <label for="confirm-password" class="ml-1 text-xs font-semibold uppercase tracking-wider text-gray-400">确认新密码</label>
            <el-input
              id="confirm-password"
              v-model="passwordForm.confirmPassword"
              type="password"
              show-password
              placeholder="请再次输入新密码"
              :prefix-icon="Lock"
              class="input-ember"
            />
          </div>
          <button
            class="btn-ember w-full rounded-2xl px-5 py-3 text-sm font-semibold cursor-pointer"
            @click="handleUpdatePassword"
          >
            更新密码
          </button>
          <p class="text-xs leading-6 text-gray-500">
            修改密码后，控制台与 Emby 客户端使用同一套新密码重新登录。
          </p>
        </div>
      </section>
    </div>

    <section class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900">连接与绑定</h2>
        </div>
      </div>

      <div class="mt-6 grid gap-4 lg:grid-cols-2">
        <article class="rounded-2xl border border-gray-200 bg-gray-50 p-5">
          <div class="flex items-start justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-gray-100 text-gray-700">
                <el-icon><Monitor /></el-icon>
              </div>
              <div>
                <h3 class="text-sm font-semibold text-gray-900">Emby 账号</h3>
                <p class="mt-1 text-xs text-gray-500">{{ embySummary }}</p>
              </div>
            </div>
            <span
              class="inline-flex shrink-0 items-center gap-2 rounded-full px-3 py-1 text-xs font-medium"
              :class="isEmbyLinked ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'"
            >
              <span class="h-2 w-2 rounded-full" :class="isEmbyLinked ? 'bg-emerald-500' : 'bg-gray-400'"></span>
              {{ embySummary }}
            </span>
          </div>

          <div class="mt-5 flex flex-col gap-3 sm:flex-row sm:items-center">
            <code class="min-w-0 flex-1 truncate rounded-2xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-700">
              {{ user.embyId || '待激活' }}
            </code>
            <button
              v-if="user.embyId"
              type="button"
              aria-label="复制 Emby ID"
              class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-400 transition-colors hover:bg-gray-50 hover:text-ember cursor-pointer"
              @click="copyToClipboard(user.embyId)"
            >
              <el-icon><CopyDocument /></el-icon>
            </button>
          </div>

          <div v-if="authStore.isAdmin" class="mt-4 flex flex-wrap gap-2">
            <button
              v-if="!user.embyId"
              type="button"
              class="btn-ember inline-flex h-11 items-center justify-center rounded-xl px-5 text-sm cursor-pointer disabled:opacity-60"
              :disabled="bindingEmby"
              @click="handleOpenEmbyBindDialog"
            >
              关联 Emby 账号
            </button>
            <button
              v-else
              type="button"
              class="inline-flex h-11 items-center justify-center rounded-xl border border-red-200 bg-white px-5 text-sm font-semibold text-red-600 transition-colors hover:bg-red-50 cursor-pointer disabled:opacity-60"
              :disabled="unbindingEmby"
              @click="handleUnbindEmby"
            >
              {{ unbindingEmby ? '解除中...' : '解除关联' }}
            </button>
          </div>
        </article>

        <article class="rounded-2xl border border-gray-200 bg-gray-50 p-5">
          <div class="flex items-start justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-sky-50 text-sky-700">
                <el-icon><ChatDotRound /></el-icon>
              </div>
              <div>
                <h3 class="text-sm font-semibold text-gray-900">Telegram Bot</h3>
                <p class="mt-1 text-xs text-gray-500">查询账号、续期和常用指令</p>
              </div>
            </div>
            <span
              class="inline-flex shrink-0 items-center gap-2 rounded-full px-3 py-1 text-xs font-medium"
              :class="isTelegramBound ? 'bg-sky-50 text-sky-700' : 'bg-gray-100 text-gray-600'"
            >
              <span class="h-2 w-2 rounded-full" :class="isTelegramBound ? 'bg-sky-500' : 'bg-gray-400'"></span>
              {{ isTelegramBound ? '已绑定' : '未绑定' }}
            </span>
          </div>

          <div class="mt-5">
            <div v-if="isTelegramBound" class="rounded-2xl border border-sky-100 bg-sky-50 p-4">
              <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                <div>
                  <p class="text-sm font-semibold text-sky-900">当前已绑定 Telegram</p>
                  <p class="mt-1 text-xs font-mono text-sky-700">ID: {{ user.telegramId }}</p>
                </div>
                <button
                  class="rounded-2xl border border-red-200 bg-white px-4 py-2 text-sm font-semibold text-red-600 transition-colors hover:bg-red-50 cursor-pointer disabled:opacity-60"
                  :disabled="unbinding"
                  @click="handleUnbindTelegram"
                >
                  {{ unbinding ? '解绑中...' : '解除绑定' }}
                </button>
              </div>
            </div>

            <div v-else-if="telegramBindCode" class="rounded-2xl border border-sky-100 bg-sky-50 p-4">
              <p class="text-sm font-semibold text-sky-900">发送以下命令到 Bot 完成绑定</p>
              <code class="mt-3 block rounded-2xl border border-sky-200 bg-white px-4 py-4 text-center text-2xl font-bold tracking-wider text-sky-900">
                /bind {{ telegramBindCode.code }}
              </code>
              <div class="mt-3 flex flex-col gap-3 text-xs text-gray-500 sm:flex-row sm:items-center sm:justify-between">
                <span>有效至 {{ formatDateTime(telegramBindCode.expiresAt, 'time') }}</span>
                <button
                  class="font-semibold text-sky-700 transition-colors hover:text-sky-900 cursor-pointer disabled:opacity-60"
                  :disabled="generatingBindCode"
                  @click="handleGenerateBindCode"
                >
                  {{ generatingBindCode ? '生成中...' : '重新生成验证码' }}
                </button>
              </div>
            </div>

            <div v-else class="rounded-2xl border border-gray-200 bg-white p-4">
              <button
                class="rounded-xl bg-sky-600 px-5 py-3 text-sm font-semibold text-white transition-colors hover:bg-sky-700 cursor-pointer disabled:opacity-60"
                :disabled="generatingBindCode"
                @click="handleGenerateBindCode"
              >
                {{ generatingBindCode ? '生成中...' : '生成绑定验证码' }}
              </button>
            </div>
          </div>
        </article>
      </div>
    </section>

    <section class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-orange-50 text-orange-600">
            <el-icon><FolderOpened /></el-icon>
          </div>
          <div>
            <h2 class="text-lg font-semibold text-gray-900">媒体库偏好</h2>
            <p class="mt-1 text-xs text-gray-500">
              {{ mediaLibrarySettings?.planGroupName || '当前分组' }} · {{ hasMediaLibraryPreferences ? '自定义' : '跟随分组模板' }}
            </p>
          </div>
        </div>

        <div v-if="mediaLibrarySettings" class="flex flex-wrap items-center gap-2">
          <el-tag
            v-if="mediaLibrarySyncing"
            type="warning"
            effect="light"
            round
          >
            同步中
          </el-tag>
          <el-tag v-else-if="mediaLibrarySettings.policySyncStatus === 'failed'" type="danger" effect="light" round>
            同步失败
          </el-tag>
          <el-tag v-else type="success" effect="light" round>
            已同步
          </el-tag>
          <el-tag type="info" effect="light" round>
            {{ mediaLibrarySettings.enabledCount }} / {{ mediaLibrarySettings.templateCount }}
          </el-tag>
        </div>
      </div>

      <div v-loading="loadingMediaLibraries" class="mt-6">
        <EmberEmptyStateCard
          v-if="!loadingMediaLibraries && !mediaLibrarySettings"
          title="媒体库偏好不可用"
          description="当前账号暂时无法读取媒体库模板。"
          :icon="FolderOpened"
          compact
        >
          <template #actions>
            <button
              type="button"
              class="btn-ember rounded-xl px-4 py-2 text-sm font-semibold"
              @click="loadMediaLibraries"
            >
              重新加载
            </button>
          </template>
        </EmberEmptyStateCard>

        <EmberEmptyStateCard
          v-else-if="!hasMediaLibraryOptions"
          title="暂无可选媒体库"
          description="当前分组还没有配置媒体库模板。"
          :icon="FolderOpened"
          compact
        />

        <div v-else class="space-y-5">
          <div
            v-if="mediaLibrarySyncing"
            class="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700"
          >
            当前媒体库权限正在同步，完成后再修改偏好。
          </div>

          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <label
              v-for="library in mediaLibrarySettings?.libraries"
              :key="library.id"
              class="flex cursor-pointer items-start gap-3 rounded-2xl border border-gray-200 bg-gray-50 p-4 transition-colors hover:border-ember/40 hover:bg-ember/5"
              :class="!library.inGroupTemplate ? 'opacity-60' : ''"
            >
              <el-checkbox
                v-model="selectedMediaLibraryIds"
                :value="library.id"
                :disabled="!library.inGroupTemplate || mediaLibrarySyncing"
                size="large"
              />
              <span class="min-w-0">
                <span class="block truncate text-sm font-semibold text-gray-900">{{ library.name }}</span>
                <span class="mt-1 block text-xs text-gray-500">{{ formatMediaLibrarySummary(library) }}</span>
              </span>
            </label>
          </div>

          <div class="flex flex-col gap-3 sm:flex-row sm:justify-end">
            <button
              type="button"
              class="rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100 cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="resettingMediaLibraries || mediaLibrarySyncing || !hasMediaLibraryPreferences"
              @click="handleResetMediaLibraries"
            >
              {{ resettingMediaLibraries ? '恢复中...' : '恢复默认' }}
            </button>
            <button
              type="button"
              class="btn-ember rounded-xl px-5 py-2.5 text-sm font-semibold cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="savingMediaLibraries || mediaLibrarySyncing"
              @click="handleSaveMediaLibraries"
            >
              {{ savingMediaLibraries ? '保存中...' : '保存偏好' }}
            </button>
          </div>
        </div>
      </div>
    </section>

    <EmberFormDialog
      v-model="verifyDialogVisible"
      title="验证邮箱变更"
    >
      <div class="px-6 pb-2 pt-2 space-y-5">
        <p class="text-sm text-gray-600">
          已向 {{ pendingNewEmail }} 发送 6 位验证码，有效期 10 分钟。
        </p>

        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <label for="email-verify-code" class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">验证码</label>
            <button
              type="button"
              class="text-xs font-semibold text-ember transition-colors hover:text-ember/80 disabled:cursor-not-allowed disabled:text-gray-400 cursor-pointer"
              :disabled="emailCodeCountdown > 0 || sendingEmailCode"
              @click="handleResendCode"
            >
              {{ emailCodeCountdown > 0 ? `${emailCodeCountdown}s 后重发` : '重新发送' }}
            </button>
          </div>
          <el-input
            id="email-verify-code"
            v-model="verifyCode"
            placeholder="请输入 6 位验证码"
            maxlength="6"
            inputmode="numeric"
            class="input-ember"
          />
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            class="rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-100 cursor-pointer"
            @click="handleCancelEmailChange"
          >
            取消
          </button>
          <button
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70 cursor-pointer"
            :disabled="confirmingEmailChange"
            @click="handleConfirmEmailChange"
          >
            {{ confirmingEmailChange ? '确认中...' : '确认' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

    <EmberFormDialog
      v-model="embyBindDialogVisible"
      title="关联 Emby 账号"
    >
      <div class="px-6 pb-2 pt-2 space-y-5">
        <div class="space-y-2">
          <label for="emby-user-search" class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-400">搜索 Emby 用户</label>
          <div class="flex flex-col gap-3 sm:flex-row">
            <el-input
              id="emby-user-search"
              v-model="embyUserSearch"
              placeholder="输入用户名或 ID"
              autocomplete="off"
              class="input-ember sm:flex-1"
              @keyup.enter="loadAdminEmbyUsers"
            />
            <button
              type="button"
              class="btn-ember rounded-2xl px-5 py-3 text-sm font-semibold cursor-pointer disabled:opacity-60"
              :disabled="loadingEmbyUsers"
              @click="loadAdminEmbyUsers"
            >
              {{ loadingEmbyUsers ? '搜索中...' : '搜索' }}
            </button>
          </div>
        </div>

        <div
          v-if="loadingEmbyUsers"
          class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-5 text-sm text-gray-500"
        >
          正在加载 Emby 用户...
        </div>
        <div
          v-else-if="embyUserOptions.length === 0"
          class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-5 text-sm text-gray-500"
        >
          输入关键词后搜索 Emby 用户
        </div>
        <div v-else class="max-h-80 space-y-2 overflow-y-auto pr-1">
          <button
            v-for="option in embyUserOptions"
            :key="option.embyId"
            type="button"
            class="flex w-full items-center justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-70"
            :class="[
              selectedEmbyId === option.embyId
                ? 'border-ember bg-ember/5'
                : 'border-gray-200 bg-white hover:bg-gray-50',
              !option.available ? 'bg-gray-50' : ''
            ]"
            :disabled="!option.available"
            @click="handleSelectEmbyUser(option)"
          >
            <span class="min-w-0">
              <span class="block truncate text-sm font-semibold text-gray-900">{{ option.name || option.embyId }}</span>
              <span class="mt-1 block truncate font-mono text-xs text-gray-500">{{ option.embyId }}</span>
            </span>
            <span
              class="shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold"
              :class="option.boundToCurrent ? 'bg-emerald-50 text-emerald-700' : option.available ? 'bg-gray-100 text-gray-600' : 'bg-red-50 text-red-600'"
            >
              {{ option.boundToCurrent ? '当前绑定' : option.available ? '可绑定' : `已绑定 ${option.boundUsername || ''}` }}
            </span>
          </button>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            class="rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-100 cursor-pointer"
            @click="handleCancelEmbyBind"
          >
            取消
          </button>
          <button
            class="btn-ember rounded-xl px-6 py-2.5 text-sm font-semibold disabled:opacity-70 cursor-pointer"
            :disabled="bindingEmby"
            @click="handleConfirmEmbyBind"
          >
            {{ bindingEmby ? '关联中...' : '确认关联' }}
          </button>
        </div>
      </template>
    </EmberFormDialog>

  </div>
</template>
