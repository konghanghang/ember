<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getUserProfile, updateUserEmail, updateUserPassword, getEmbyConfig, getMediaStats } from '@/api/user'
import type { MediaStats, UserInfo } from '@/types/api'

const user = ref<UserInfo>({
  id: '',
  username: '',
  email: '',
  embyId: '',
  expiresAt: '',
  isActive: false,
  createdAt: ''
})
const embyUrl = ref('')
const stats = ref<MediaStats>({
  MovieCount: 0,
  SeriesCount: 0,
  EpisodeCount: 0
})
const loading = ref(false)

// Password Form
const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const fetchProfile = async () => {
  loading.value = true
  try {
    const res = await getUserProfile()
    user.value = res
  } finally {
    loading.value = false
  }
}

const fetchMediaInfo = async () => {
  const configRes = await getEmbyConfig()
  if (configRes.success) embyUrl.value = configRes.url

  const statsRes = await getMediaStats()
  if (statsRes.success) stats.value = statsRes.data
}

const handleUpdateEmail = async () => {
  try {
    await updateUserEmail(user.value.email)
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
    await updateUserPassword({
      oldPassword: passwordForm.value.oldPassword,
      newPassword: passwordForm.value.newPassword
    })
    ElMessage.success('密码修改成功')
    passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  } catch {
    ElMessage.error('密码修改失败，请稍后重试')
  }
}

onMounted(() => {
  fetchProfile()
  fetchMediaInfo()
})
</script>

<template>
  <div class="max-w-6xl mx-auto py-8">
    <div class="mb-8">
      <h1 class="text-2xl font-bold text-primary mb-2">欢迎回来, {{ user.username }}</h1>
      <p class="text-secondary">管理您的 Emby 账号和订阅</p>
    </div>

    <!-- Media Stats -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
      <div class="panel-clean p-6 flex items-center justify-between border-l-4 border-l-purple-500">
        <div>
          <div class="text-secondary text-sm mb-1">电影收藏</div>
          <div class="text-3xl font-bold text-primary">{{ stats.MovieCount || 0 }}</div>
        </div>
        <div class="bg-purple-50 p-3 rounded-full">
          <i class="el-icon-film text-purple-500 text-xl"></i>
        </div>
      </div>
      
      <div class="panel-clean p-6 flex items-center justify-between border-l-4 border-l-green-500">
        <div>
          <div class="text-secondary text-sm mb-1">剧集收藏</div>
          <div class="text-3xl font-bold text-primary">{{ stats.SeriesCount || 0 }}</div>
        </div>
        <div class="bg-green-50 p-3 rounded-full">
          <i class="el-icon-monitor text-green-500 text-xl"></i>
        </div>
      </div>

      <div class="panel-clean p-6 flex items-center justify-between border-l-4 border-l-ember">
        <div>
          <div class="text-secondary text-sm mb-1">单集总数</div>
          <div class="text-3xl font-bold text-primary">{{ stats.EpisodeCount || 0 }}</div>
        </div>
        <div class="bg-ember/10 p-3 rounded-full">
          <i class="el-icon-video-play text-ember text-xl"></i>
        </div>
      </div>
    </div>

    <el-alert
      v-if="embyUrl"
      :title="`Emby 服务器地址: ${embyUrl}`"
      type="success"
      :closable="false"
      class="mb-8"
      show-icon
    />

    <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
      <!-- Profile Info -->
      <div class="panel-clean p-6">
        <h3 class="text-lg font-bold text-primary mb-6 border-b border-gray-100 pb-4">账号信息</h3>
        <el-form label-position="top" class="custom-form">
          <el-form-item label="用户名">
            <el-input v-model="user.username" disabled class="input-ember" />
          </el-form-item>
          <el-form-item label="Emby ID">
            <el-input v-model="user.embyId" disabled class="input-ember" />
          </el-form-item>
          <el-form-item label="到期时间">
            <el-input :value="new Date(user.expiresAt).toLocaleDateString()" disabled class="input-ember" />
          </el-form-item>
          <el-form-item label="邮箱">
            <div class="flex w-full gap-4">
              <el-input v-model="user.email" placeholder="输入新邮箱" class="input-ember flex-1" />
              <button class="btn-ember px-4 py-2 rounded-lg text-sm" @click="handleUpdateEmail">
                更新
              </button>
            </div>
          </el-form-item>
        </el-form>
      </div>
      
      <!-- Change Password -->
      <div class="panel-clean p-6">
        <h3 class="text-lg font-bold text-primary mb-6 border-b border-gray-100 pb-4">安全设置</h3>
        <el-form label-position="top" class="custom-form">
          <el-form-item label="旧密码">
            <el-input v-model="passwordForm.oldPassword" type="password" show-password class="input-ember" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="passwordForm.newPassword" type="password" show-password class="input-ember" />
          </el-form-item>
          <el-form-item label="确认密码">
            <el-input v-model="passwordForm.confirmPassword" type="password" show-password class="input-ember" />
          </el-form-item>
          <el-form-item class="mt-8">
            <button class="btn-ember w-full py-2.5 rounded-lg font-medium" @click="handleUpdatePassword">
              修改密码
            </button>
          </el-form-item>
        </el-form>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 覆盖 Element Plus 默认样式以匹配设计 */
:deep(.el-form-item__label) {
  color: var(--color-text-secondary);
  font-weight: 500;
}

:deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px #e5e7eb inset;
  background-color: #f9fafb;
}

:deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 2px var(--color-ember) inset !important;
  background-color: white;
}
</style>
