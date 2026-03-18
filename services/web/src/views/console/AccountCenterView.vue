<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Bell,
  ChatDotRound,
  Check,
  CopyDocument,
  Key,
  Lock,
  Message,
  Monitor,
  Reading,
  UserFilled
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { useConsoleStore } from '@/store/console'
import { useUserStore } from '@/store/user'
import {
  generateTelegramBindCode,
  unbindTelegram,
  updatePassword
} from '@/api/console'
import type { ConsoleAccountLinkIcon, TelegramBindCodeResponse, UserInfo } from '@/types/api'

const authStore = useAuthStore()
const consoleStore = useConsoleStore()
const userStore = useUserStore()
const router = useRouter()

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

const resourceIconMap: Record<ConsoleAccountLinkIcon, Component> = {
  notify: Bell,
  group: ChatDotRound,
  wiki: Reading
}

const isExpired = computed(() => {
  if (!user.value.expiresAt) return false
  return new Date(user.value.expiresAt) < new Date()
})

const isTelegramBound = computed(() => !!user.value.telegramId)

const membershipSummary = computed(() => {
  if (authStore.isAdmin) return '管理员账号'
  return isExpired.value ? '会员已过期' : '会员有效'
})

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

const handleUpdateEmail = async () => {
  loading.value = true
  try {
    await userStore.updateEmail(emailInput.value.trim())
    ElMessage.success('邮箱更新成功')
  } catch {
    // handled
  } finally {
    loading.value = false
  }
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
</script>

<template>
  <div class="space-y-6" v-loading="loading">
    <section class="overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-sm">
      <div class="border-b border-slate-100 bg-slate-50/80 px-6 py-4">
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">Account Center</p>
      </div>
      <div class="grid gap-6 px-6 py-6 lg:grid-cols-[1.6fr_1fr] lg:px-8">
        <div class="flex items-start gap-4">
          <div class="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-slate-900 to-slate-700 text-2xl font-bold text-white shadow-sm">
            {{ (user.username || 'U').charAt(0).toUpperCase() }}
          </div>
          <div class="space-y-3">
            <div>
              <div class="flex flex-wrap items-center gap-3">
                <h1 class="text-2xl font-semibold text-slate-900">{{ user.username || '当前用户' }}</h1>
                <span class="rounded-full bg-ember/10 px-3 py-1 text-xs font-semibold text-ember">
                  {{ authStore.isAdmin ? '管理员' : '普通用户' }}
                </span>
              </div>
            </div>

            <div class="flex flex-wrap gap-2">
              <span
                class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium"
                :class="isExpired ? 'bg-red-50 text-red-700' : 'bg-emerald-50 text-emerald-700'"
              >
                <span class="h-2 w-2 rounded-full" :class="isExpired ? 'bg-red-500' : 'bg-emerald-500'"></span>
                {{ membershipSummary }}
              </span>
              <span
                class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium"
                :class="isTelegramBound ? 'bg-sky-50 text-sky-700' : 'bg-slate-100 text-slate-600'"
              >
                <el-icon><Check /></el-icon>
                {{ isTelegramBound ? 'Telegram 已绑定' : 'Telegram 未绑定' }}
              </span>
            </div>
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
          <button
            class="flex items-center justify-between rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 text-left transition-colors hover:border-slate-300 hover:bg-white cursor-pointer"
            @click="router.push('/console/dashboard')"
          >
            <div>
              <p class="text-sm font-semibold text-slate-900">回到概览</p>
              <p class="mt-1 text-xs text-slate-500">查看会员状态、媒体统计和服务器入口</p>
            </div>
            <el-icon class="text-slate-400"><Monitor /></el-icon>
          </button>
        </div>
      </div>
    </section>

    <div class="grid gap-6 xl:grid-cols-[1.2fr_1fr]">
      <section class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-amber-50 text-amber-600">
            <el-icon><UserFilled /></el-icon>
          </div>
          <div>
            <h2 class="text-lg font-semibold text-slate-900">基本信息</h2>
          </div>
        </div>

        <div class="mt-6 grid gap-5 md:grid-cols-2">
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">用户名</p>
            <div class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
              {{ user.username || '-' }}
            </div>
          </div>
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Emby ID</p>
            <div class="flex items-center gap-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
              <span class="min-w-0 flex-1 truncate font-mono">{{ user.embyId || '待激活' }}</span>
              <button
                v-if="user.embyId"
                class="text-slate-400 transition-colors hover:text-ember cursor-pointer"
                @click="copyToClipboard(user.embyId)"
              >
                <el-icon><CopyDocument /></el-icon>
              </button>
            </div>
          </div>
          <div class="space-y-2 md:col-span-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">联系邮箱</p>
            <div class="flex flex-col gap-3 sm:flex-row">
              <el-input
                v-model="emailInput"
                placeholder="name@example.com"
                :prefix-icon="Message"
                class="input-ember sm:flex-1"
              />
              <button
                class="btn-ember rounded-2xl px-5 py-3 text-sm font-semibold cursor-pointer"
                @click="handleUpdateEmail"
              >
                保存邮箱
              </button>
            </div>
            <p class="text-xs text-slate-500">用于找回密码和接收系统通知。</p>
          </div>
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">有效期</p>
            <div class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
              {{ user.expiresAt ? new Date(user.expiresAt).toLocaleString() : '永久有效' }}
            </div>
          </div>
          <div class="space-y-2">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">创建时间</p>
            <div class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
              {{ user.createdAt ? new Date(user.createdAt).toLocaleString() : '-' }}
            </div>
          </div>
        </div>
      </section>

      <section class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-slate-100 text-slate-700">
            <el-icon><Key /></el-icon>
          </div>
          <div>
            <h2 class="text-lg font-semibold text-slate-900">安全设置</h2>
          </div>
        </div>

        <div class="mt-6 space-y-4">
          <div class="space-y-1">
            <label class="ml-1 text-xs font-semibold uppercase tracking-wider text-slate-400">当前密码</label>
            <el-input
              v-model="passwordForm.oldPassword"
              type="password"
              show-password
              placeholder="请输入当前密码"
              :prefix-icon="Lock"
              class="input-ember"
            />
          </div>
          <div class="space-y-1">
            <label class="ml-1 text-xs font-semibold uppercase tracking-wider text-slate-400">新密码</label>
            <el-input
              v-model="passwordForm.newPassword"
              type="password"
              show-password
              placeholder="请输入新密码"
              :prefix-icon="Lock"
              class="input-ember"
            />
          </div>
          <div class="space-y-1">
            <label class="ml-1 text-xs font-semibold uppercase tracking-wider text-slate-400">确认新密码</label>
            <el-input
              v-model="passwordForm.confirmPassword"
              type="password"
              show-password
              placeholder="请再次输入新密码"
              :prefix-icon="Lock"
              class="input-ember"
            />
          </div>
          <button
            class="w-full rounded-2xl bg-ember px-5 py-3 text-sm font-semibold text-white transition-colors hover:bg-red-600 cursor-pointer"
            @click="handleUpdatePassword"
          >
            更新密码
          </button>
          <p class="text-xs leading-6 text-slate-500">
            修改密码后，控制台与 Emby 客户端使用同一套新密码重新登录。
          </p>
        </div>
      </section>
    </div>

    <section class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-slate-900">Telegram 绑定</h2>
          <p class="text-sm text-slate-500">绑定后可在 Bot 中查询账号信息、续期和处理常用指令。</p>
        </div>
        <span
          class="inline-flex items-center gap-2 self-start rounded-full px-3 py-1 text-xs font-medium"
          :class="isTelegramBound ? 'bg-sky-50 text-sky-700' : 'bg-slate-100 text-slate-600'"
        >
          <span class="h-2 w-2 rounded-full" :class="isTelegramBound ? 'bg-sky-500' : 'bg-slate-400'"></span>
          {{ isTelegramBound ? '已绑定' : '未绑定' }}
        </span>
      </div>

      <div class="mt-6">
        <div v-if="isTelegramBound" class="rounded-2xl border border-sky-100 bg-sky-50 p-5">
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

        <div v-else-if="telegramBindCode" class="rounded-2xl border border-sky-100 bg-sky-50 p-5">
          <p class="text-sm font-semibold text-sky-900">发送以下命令到 Bot 完成绑定</p>
          <code class="mt-3 block rounded-2xl border border-sky-200 bg-white px-4 py-4 text-center text-2xl font-bold tracking-wider text-sky-900">
            /bind {{ telegramBindCode.code }}
          </code>
          <div class="mt-3 flex flex-col gap-3 text-xs text-slate-500 sm:flex-row sm:items-center sm:justify-between">
            <span>有效至 {{ new Date(telegramBindCode.expiresAt).toLocaleTimeString() }}</span>
            <button
              class="font-semibold text-sky-700 transition-colors hover:text-sky-900 cursor-pointer disabled:opacity-60"
              :disabled="generatingBindCode"
              @click="handleGenerateBindCode"
            >
              {{ generatingBindCode ? '生成中...' : '重新生成验证码' }}
            </button>
          </div>
        </div>

        <div v-else class="rounded-2xl border border-slate-200 bg-slate-50 p-5">
          <p class="text-sm text-slate-700">
            绑定后可在 Telegram Bot 中使用 `/info` 查询账号信息，使用 `/redeem` 兑换续期码。
          </p>
          <button
            class="mt-4 rounded-2xl bg-sky-600 px-5 py-3 text-sm font-semibold text-white transition-colors hover:bg-sky-700 cursor-pointer disabled:opacity-60"
            :disabled="generatingBindCode"
            @click="handleGenerateBindCode"
          >
            {{ generatingBindCode ? '生成中...' : '生成绑定验证码' }}
          </button>
        </div>
      </div>
    </section>

  </div>
</template>
