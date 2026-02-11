<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'

const router = useRouter()
const authStore = useAuthStore()
const form = ref({
  username: '',
  password: '',
  email: '',
  inviteCode: ''
})
const loading = ref(false)

const handleRegister = async () => {
  if (!form.value.username || !form.value.password || !form.value.inviteCode) {
    ElMessage.warning('请填写必填项')
    return
  }
  
  loading.value = true
  try {
    await authStore.register(form.value)
    ElMessage.success('注册成功')
    router.push('/user/dashboard')
  } catch (error) {
    // Error handled in interceptor
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-cinema-bg px-4">
    <div class="w-full max-w-md">
      <div class="panel-clean rounded-2xl p-8">
        <!-- Logo -->
        <div class="text-center mb-8">
          <h1 class="text-3xl font-bold text-ember">
            Ember
          </h1>
          <p class="text-text-secondary mt-2">
            用户注册
          </p>
        </div>

        <!-- Register Form -->
        <el-form :model="form" @submit.prevent="handleRegister" size="large" label-position="top">
          <el-form-item label="邀请码" required>
            <el-input v-model="form.inviteCode" placeholder="请输入邀请码" class="input-ember" />
          </el-form-item>
          <el-form-item label="用户名" required>
            <el-input v-model="form.username" placeholder="3-20位字符" class="input-ember" />
          </el-form-item>
          <el-form-item label="密码" required>
            <el-input v-model="form.password" type="password" placeholder="至少6位" show-password class="input-ember" />
          </el-form-item>
          <el-form-item label="邮箱">
            <el-input v-model="form.email" placeholder="用于接收通知" class="input-ember" />
          </el-form-item>
          <el-form-item class="mt-6">
            <el-button native-type="submit" :loading="loading" class="btn-ember w-full">
              注册
            </el-button>
          </el-form-item>
        </el-form>

        <!-- Footer Links -->
        <div class="mt-6 text-center">
          <p class="text-sm text-text-secondary mb-4">
            注册成功后，使用用户名和密码登录 Emby 服务器
          </p>
          <router-link to="/" class="text-sm text-text-secondary hover:text-ember transition-colors">
            ← 返回首页
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>
