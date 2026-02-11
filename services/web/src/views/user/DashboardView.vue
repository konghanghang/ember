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
  <div class="dashboard-container">
    <!-- Media Stats -->
    <el-row :gutter="20" class="mb-20">
      <el-col :span="8">
        <el-card shadow="always" class="stat-card movie">
          <div class="stat-content">
            <div class="stat-label">电影</div>
            <div class="stat-num">{{ stats.MovieCount || 0 }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="always" class="stat-card tv">
          <div class="stat-content">
            <div class="stat-label">剧集</div>
            <div class="stat-num">{{ stats.SeriesCount || 0 }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="always" class="stat-card episode">
          <div class="stat-content">
            <div class="stat-label">单集</div>
            <div class="stat-num">{{ stats.EpisodeCount || 0 }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-alert
      v-if="embyUrl"
      :title="`Emby 服务器地址: ${embyUrl}`"
      type="info"
      :closable="false"
      class="mb-20"
      show-icon
    />

    <el-row :gutter="20">
      <el-col :span="12">
        <el-card header="账号信息">
          <el-form label-width="80px">
            <el-form-item label="用户名">
              <el-input v-model="user.username" disabled />
            </el-form-item>
            <el-form-item label="Emby ID">
              <el-input v-model="user.embyId" disabled />
            </el-form-item>
            <el-form-item label="到期时间">
              <el-input :value="new Date(user.expiresAt).toLocaleDateString()" disabled />
            </el-form-item>
            <el-form-item label="邮箱">
              <el-input v-model="user.email" placeholder="输入新邮箱" />
              <el-button type="primary" link @click="handleUpdateEmail" style="margin-left: 10px">更新</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
      
      <el-col :span="12">
        <el-card header="修改密码">
          <el-form label-width="80px">
            <el-form-item label="旧密码">
              <el-input v-model="passwordForm.oldPassword" type="password" show-password />
            </el-form-item>
            <el-form-item label="新密码">
              <el-input v-model="passwordForm.newPassword" type="password" show-password />
            </el-form-item>
            <el-form-item label="确认密码">
              <el-input v-model="passwordForm.confirmPassword" type="password" show-password />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="handleUpdatePassword">修改密码</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.dashboard-container {
  max-width: 1200px;
  margin: 0 auto;
}
.mb-20 {
  margin-bottom: 20px;
}
.stat-card {
  color: white;
}
.stat-card.movie { background: linear-gradient(135deg, #a855f7, #9333ea); }
.stat-card.tv { background: linear-gradient(135deg, #22c55e, #16a34a); }
.stat-card.episode { background: linear-gradient(135deg, #f97316, #ea580c); }

.stat-content {
  text-align: center;
}
.stat-label {
  font-size: 14px;
  opacity: 0.9;
}
.stat-num {
  font-size: 32px;
  font-weight: bold;
}
</style>
