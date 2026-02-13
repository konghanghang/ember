<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Key, Lock, Message, Ticket, User } from '@element-plus/icons-vue'
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
    ElMessage.error('邮箱更新失败，请稍后重试')
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
    ElMessage.error('密码修改失败，请稍后重试')
  }
}

onMounted(refreshAll)
</script>

<template>
  <div class="max-w-6xl mx-auto py-8" v-loading="loading">
    <div class="mb-8">
      <h1 class="text-2xl font-bold text-primary mb-2">欢迎回来, {{ user.username }}</h1>
      <p class="text-secondary">管理您的 Emby 账号和订阅</p>
    </div>

    <el-alert
      v-if="authStore.isAdmin"
      title="管理员账号"
      type="info"
      :closable="false"
      class="mb-6"
      show-icon
    />
    <el-alert
      v-else-if="isExpired"
      title="你的账号已过期，Emby 访问已暂停。请使用兑换码续期。"
      type="warning"
      :closable="false"
      class="mb-6"
      show-icon
    />
    <el-alert
      v-else
      :title="`有效期至 ${user.expiresAt ? new Date(user.expiresAt).toLocaleDateString() : '-'}（剩余 ${daysLeft} 天）`"
      type="success"
      :closable="false"
      class="mb-6"
      show-icon
    />

    <div v-if="!authStore.isAdmin && isExpired" class="panel-clean p-6 mb-8 border-l-4 border-l-red-500">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-lg font-semibold">账号续期</h3>
      </div>
      <div class="flex gap-3">
        <el-input v-model="redeemForm.code" placeholder="请输入兑换码" class="input-ember" :prefix-icon="Ticket" />
        <el-button type="primary" :loading="redeeming" @click="handleRedeem">兑换</el-button>
      </div>
    </div>

    <el-collapse v-else-if="!authStore.isAdmin" class="mb-8">
      <el-collapse-item title="提前续期" name="1">
        <div class="flex gap-3">
          <el-input v-model="redeemForm.code" placeholder="请输入兑换码" class="input-ember" :prefix-icon="Ticket" />
          <el-button type="primary" :loading="redeeming" @click="handleRedeem">兑换</el-button>
        </div>
      </el-collapse-item>
    </el-collapse>

    <div class="mb-8">
      <div class="relative" :class="{ 'opacity-40 pointer-events-none': !authStore.isAdmin && isExpired }">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div class="panel-clean p-6 flex items-center justify-between border-l-4 border-l-purple-500">
            <div>
              <div class="text-secondary text-sm mb-1">电影收藏</div>
              <div class="text-3xl font-bold text-primary">{{ stats.MovieCount || 0 }}</div>
            </div>
          </div>

          <div class="panel-clean p-6 flex items-center justify-between border-l-4 border-l-green-500">
            <div>
              <div class="text-secondary text-sm mb-1">剧集收藏</div>
              <div class="text-3xl font-bold text-primary">{{ stats.SeriesCount || 0 }}</div>
            </div>
          </div>

          <div class="panel-clean p-6 flex items-center justify-between border-l-4 border-l-ember">
            <div>
              <div class="text-secondary text-sm mb-1">单集总数</div>
              <div class="text-3xl font-bold text-primary">{{ stats.EpisodeCount || 0 }}</div>
            </div>
          </div>
        </div>
      </div>
      <div v-if="!authStore.isAdmin && isExpired" class="text-center text-sm text-text-secondary mt-3">🔒 续期后自动恢复</div>
    </div>

    <el-alert v-if="embyUrl && (!isExpired || authStore.isAdmin)" :title="`Emby 服务器地址: ${embyUrl}`" type="success" :closable="false" class="mb-8" show-icon />

    <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
      <div class="panel-clean p-6">
        <h3 class="text-lg font-bold text-primary mb-6 border-b border-gray-100 pb-4">账号信息</h3>
        <el-form label-position="top" class="custom-form">
          <el-form-item label="用户名">
            <el-input v-model="user.username" disabled class="input-ember" :prefix-icon="User" />
          </el-form-item>
          <el-form-item label="Emby ID">
            <el-input v-model="user.embyId" disabled class="input-ember" :prefix-icon="Key" />
          </el-form-item>
          <el-form-item label="到期时间">
            <el-tag :type="expiryTagType as any">
              {{ user.expiresAt ? new Date(user.expiresAt).toLocaleString() : '-' }}
            </el-tag>
          </el-form-item>
          <el-form-item label="邮箱">
            <div class="flex w-full gap-4">
              <el-input v-model="user.email" placeholder="输入新邮箱" class="input-ember flex-1" :prefix-icon="Message" />
              <button type="button" class="btn-ember px-4 py-2 rounded-lg text-sm" @click="handleUpdateEmail">更新</button>
            </div>
          </el-form-item>
        </el-form>
      </div>

      <div class="panel-clean p-6">
        <h3 class="text-lg font-bold text-primary mb-6 border-b border-gray-100 pb-4">安全设置</h3>
        <el-form label-position="top" class="custom-form">
          <el-form-item label="旧密码">
            <el-input v-model="passwordForm.oldPassword" type="password" show-password class="input-ember" :prefix-icon="Lock" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="passwordForm.newPassword" type="password" show-password class="input-ember" :prefix-icon="Lock" />
          </el-form-item>
          <el-form-item label="确认密码">
            <el-input v-model="passwordForm.confirmPassword" type="password" show-password class="input-ember" :prefix-icon="Lock" />
          </el-form-item>
          <el-form-item class="mt-8">
            <button type="button" class="btn-ember w-full py-2.5 rounded-lg font-medium" @click="handleUpdatePassword">修改密码</button>
          </el-form-item>
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
