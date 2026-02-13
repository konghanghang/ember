<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Key, Lock, Message, Ticket, User, CircleCloseFilled, Film, Top, Monitor, InfoFilled, CopyDocument, VideoPlay } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { getEmbyConfig, getMediaStats, getProfile, updateEmail, updatePassword } from '@/api/console'
import { redeemCode } from '@/api/user'
import type { MediaStats, UserInfo } from '@/types/api'

const authStore = useAuthStore()

const user = ref<UserInfo>({
  id: '',
  username: '',
  role: 'user',
  email: '',
  embyId: '',
  expiresAt: '',
  embyDisabled: false,
  isActive: false,
  createdAt: ''
})

const embyUrl = ref('')
const stats = ref<MediaStats>({ MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 })
const loading = ref(false)
const redeeming = ref(false)

const redeemForm = ref({ code: '' })

const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const isExpired = computed(() => {
  if (!user.value.expiresAt) return false
  return new Date(user.value.expiresAt) < new Date()
})

const daysLeft = computed(() => {
  if (!user.value.expiresAt) return 0
  const ms = new Date(user.value.expiresAt).getTime() - Date.now()
  return Math.ceil(ms / (24 * 60 * 60 * 1000))
})

const expiryTagType = computed(() => {
  if (!user.value.expiresAt) return 'info'
  if (isExpired.value) return 'danger'
  if (daysLeft.value < 7) return 'warning'
  return 'success'
})

const fetchProfile = async () => {
  loading.value = true
  try {
    user.value = await getProfile()
  } finally {
    loading.value = false
  }
}

const fetchMediaInfo = async () => {
  if (!authStore.isAdmin && isExpired.value) return
  const [configRes, statsRes] = await Promise.all([getEmbyConfig(), getMediaStats()])
  if (configRes.success) embyUrl.value = configRes.url
  if (statsRes.success) stats.value = statsRes.data
}

const refreshAll = async () => {
  await fetchProfile()
  await fetchMediaInfo()
}

const handleRedeem = async () => {
  if (!redeemForm.value.code) {
    ElMessage.warning('请输入兑换码')
    return
  }

  redeeming.value = true
  try {
    const res = await redeemCode({ code: redeemForm.value.code })
    ElMessage.success(res.message)
    redeemForm.value.code = ''
    await refreshAll()
  } finally {
    redeeming.value = false
  }
}

const handleUpdateEmail = async () => {
  try {
    await updateEmail(user.value.email || '')
    ElMessage.success('邮箱更新成功')
  } catch {
    // 错误提示由全局请求拦截器统一处理，避免重复弹窗
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
    // 错误提示由全局请求拦截器统一处理，避免重复弹窗
  }
}

onMounted(refreshAll)
</script>

<template>
  <div class="space-y-6" v-loading="loading">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">欢迎回来, {{ user.username }}</h1>
        <p class="text-gray-500 mt-1">管理您的 Emby 账号和订阅状态</p>
      </div>
      <div class="flex items-center gap-3">
        <span 
          class="inline-flex items-center px-3 py-1 rounded-full text-xs font-medium ring-1 ring-inset"
          :class="{
            'bg-green-50 text-green-700 ring-green-600/20': !isExpired && !authStore.isAdmin,
            'bg-red-50 text-red-700 ring-red-600/20': isExpired && !authStore.isAdmin,
            'bg-blue-50 text-blue-700 ring-blue-600/20': authStore.isAdmin
          }"
        >
          {{ authStore.isAdmin ? '管理员' : (isExpired ? '已过期' : '订阅有效') }}
        </span>
        <span v-if="!authStore.isAdmin && user.expiresAt" class="text-sm text-gray-500">
          有效期至 {{ new Date(user.expiresAt).toLocaleDateString() }}
        </span>
      </div>
    </div>

    <!-- Alerts / Banners -->
    <div v-if="!authStore.isAdmin && isExpired" class="bg-red-50 border-l-4 border-red-500 p-4 rounded-r-lg shadow-sm">
      <div class="flex">
        <div class="flex-shrink-0">
          <el-icon class="text-red-400" :size="20"><CircleCloseFilled /></el-icon>
        </div>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-red-800">账号已过期</h3>
          <div class="mt-2 text-sm text-red-700">
            <p>您的 Emby 访问权限已暂停。请使用兑换码续期以恢复服务。</p>
          </div>
          <div class="mt-4">
            <div class="flex items-center gap-3 max-w-md">
              <el-input 
                v-model="redeemForm.code" 
                placeholder="输入兑换码" 
                class="input-ember" 
                :prefix-icon="Ticket"
              />
              <button 
                @click="handleRedeem" 
                :disabled="redeeming"
                class="px-4 py-2 bg-red-600 text-white text-sm font-medium rounded-md hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 transition-colors"
              >
                {{ redeeming ? '兑换中...' : '立即续期' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Actions (Renew for active users) -->
    <div v-else-if="!authStore.isAdmin" class="bg-white border border-gray-100 rounded-xl p-6 shadow-sm flex flex-col md:flex-row items-center justify-between gap-4">
      <div class="flex items-center gap-4">
        <div class="w-10 h-10 rounded-full bg-ember/10 flex items-center justify-center text-ember">
          <el-icon :size="20"><Ticket /></el-icon>
        </div>
        <div>
          <h3 class="text-sm font-semibold text-gray-900">账号续期</h3>
          <p class="text-xs text-gray-500">使用兑换码延长您的订阅时间</p>
        </div>
      </div>
      <div class="flex items-center gap-3 w-full md:w-auto">
        <el-input 
          v-model="redeemForm.code" 
          placeholder="输入兑换码" 
          class="input-ember w-full md:w-64"
          :prefix-icon="Ticket"
        />
        <button 
          @click="handleRedeem" 
          :disabled="redeeming"
          class="whitespace-nowrap px-4 py-2 bg-white border border-gray-300 text-gray-700 text-sm font-medium rounded-lg hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-ember transition-colors"
        >
          {{ redeeming ? '...' : '兑换' }}
        </button>
      </div>
    </div>

    <!-- Stats Grid -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="bg-white rounded-xl p-6 border border-gray-100 shadow-sm relative overflow-hidden group hover:shadow-md transition-shadow">
        <div class="absolute top-0 right-0 p-4 opacity-10 group-hover:opacity-20 transition-opacity">
          <el-icon :size="64" class="text-purple-500"><Film /></el-icon>
        </div>
        <div class="relative z-10">
          <p class="text-sm font-medium text-gray-500">电影收藏</p>
          <p class="text-3xl font-bold text-gray-900 mt-2">{{ stats.MovieCount || 0 }}</p>
          <div class="mt-4 flex items-center text-xs text-green-600 bg-green-50 w-fit px-2 py-1 rounded-full">
            <el-icon class="mr-1"><Top /></el-icon>
            <span>实时更新</span>
          </div>
        </div>
      </div>

      <div class="bg-white rounded-xl p-6 border border-gray-100 shadow-sm relative overflow-hidden group hover:shadow-md transition-shadow">
        <div class="absolute top-0 right-0 p-4 opacity-10 group-hover:opacity-20 transition-opacity">
          <el-icon :size="64" class="text-green-500"><VideoPlay /></el-icon>
        </div>
        <div class="relative z-10">
          <p class="text-sm font-medium text-gray-500">剧集收藏</p>
          <p class="text-3xl font-bold text-gray-900 mt-2">{{ stats.SeriesCount || 0 }}</p>
          <div class="mt-4 flex items-center text-xs text-green-600 bg-green-50 w-fit px-2 py-1 rounded-full">
            <el-icon class="mr-1"><Top /></el-icon>
            <span>海量资源</span>
          </div>
        </div>
      </div>

      <div class="bg-white rounded-xl p-6 border border-gray-100 shadow-sm relative overflow-hidden group hover:shadow-md transition-shadow">
        <div class="absolute top-0 right-0 p-4 opacity-10 group-hover:opacity-20 transition-opacity">
          <el-icon :size="64" class="text-ember"><Monitor /></el-icon>
        </div>
        <div class="relative z-10">
          <p class="text-sm font-medium text-gray-500">总集数</p>
          <p class="text-3xl font-bold text-gray-900 mt-2">{{ stats.EpisodeCount || 0 }}</p>
          <div class="mt-4 flex items-center text-xs text-blue-600 bg-blue-50 w-fit px-2 py-1 rounded-full">
            <el-icon class="mr-1"><InfoFilled /></el-icon>
            <span>持续收录</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Emby Server Info -->
    <div v-if="embyUrl && (!isExpired || authStore.isAdmin)" class="bg-gradient-to-r from-gray-900 to-gray-800 rounded-xl p-6 text-white shadow-lg relative overflow-hidden">
      <div class="absolute top-0 right-0 w-64 h-64 bg-white opacity-5 rounded-full -translate-y-1/2 translate-x-1/3 blur-3xl"></div>
      <div class="relative z-10 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div class="flex items-center gap-4">
          <div class="w-12 h-12 rounded-lg bg-white/10 flex items-center justify-center backdrop-blur-sm">
            <el-icon :size="24"><Monitor /></el-icon>
          </div>
          <div>
            <h3 class="text-lg font-bold">Emby 服务器地址</h3>
            <p class="text-gray-400 text-sm mt-1">请使用此地址在客户端登录</p>
          </div>
        </div>
        <div class="flex items-center gap-2 bg-black/30 px-4 py-2 rounded-lg backdrop-blur-sm border border-white/10">
          <code class="text-green-400 font-mono">{{ embyUrl }}</code>
          <button class="text-gray-400 hover:text-white transition-colors">
            <el-icon><CopyDocument /></el-icon>
          </button>
        </div>
      </div>
    </div>

    <!-- Settings Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
      <!-- Account Info -->
      <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-lg font-bold text-gray-900">基本信息</h3>
          <el-tag size="small" effect="plain">只读</el-tag>
        </div>
        
        <el-form label-position="top" class="space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="用户名">
              <el-input v-model="user.username" disabled class="input-ember" :prefix-icon="User" />
            </el-form-item>
            <el-form-item label="Emby ID">
              <el-input v-model="user.embyId" disabled class="input-ember" :prefix-icon="Key" />
            </el-form-item>
          </div>
          
          <el-form-item label="注册邮箱">
            <div class="flex w-full gap-3">
              <el-input v-model="user.email" placeholder="输入新邮箱" class="input-ember flex-1" :prefix-icon="Message" />
              <button 
                type="button"
                @click="handleUpdateEmail"
                class="px-4 py-2 bg-gray-900 text-white text-sm font-medium rounded-lg hover:bg-gray-800 transition-colors"
              >
                更新
              </button>
            </div>
            <p class="text-xs text-gray-400 mt-1">用于找回密码和接收通知</p>
          </el-form-item>
        </el-form>
      </div>

      <!-- Security Settings -->
      <div class="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-lg font-bold text-gray-900">安全设置</h3>
          <el-icon class="text-gray-400"><Lock /></el-icon>
        </div>

        <el-form label-position="top" class="space-y-4">
          <el-form-item label="当前密码">
            <el-input
              v-model="passwordForm.oldPassword"
              type="password"
              show-password
              class="input-ember"
              placeholder="输入当前密码"
              :prefix-icon="Lock"
            />
          </el-form-item>
          
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="新密码">
              <el-input
                v-model="passwordForm.newPassword"
                type="password"
                show-password
                class="input-ember"
                placeholder="设置新密码"
                :prefix-icon="Lock"
              />
            </el-form-item>
            <el-form-item label="确认密码">
              <el-input
                v-model="passwordForm.confirmPassword"
                type="password"
                show-password
                class="input-ember"
                placeholder="重复新密码"
                :prefix-icon="Lock"
              />
            </el-form-item>
          </div>

          <button 
            type="button" 
            @click="handleUpdatePassword"
            class="w-full mt-2 py-2.5 bg-ember text-white font-medium rounded-lg hover:bg-red-700 active:scale-[0.99] transition-all shadow-sm shadow-red-200"
          >
            修改密码
          </button>
        </el-form>
      </div>
    </div>
  </div>
</template>

<style scoped>
:deep(.el-form-item__label) {
  color: var(--text-secondary);
  font-weight: 500;
}

:deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px #e5e7eb inset;
  background-color: #f9fafb;
}

:deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 2px var(--ember-red) inset !important;
  background-color: white;
}
</style>
