<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'

const router = useRouter()
const authStore = useAuthStore()
const form = ref({
  username: '',
  password: ''
})
const loading = ref(false)

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  
  loading.value = true
  try {
    await authStore.adminLogin(form.value)
    ElMessage.success('登录成功')
    router.push('/admin/users')
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
            管理员登录
          </p>
        </div>

        <!-- Login Form -->
        <el-form :model="form" @submit.prevent="handleLogin" size="large">
          <el-form-item>
            <el-input v-model="form.username" placeholder="请输入用户名" prefix-icon="User" class="input-ember" />
          </el-form-item>
          <el-form-item>
            <el-input v-model="form.password" type="password" placeholder="请输入密码" prefix-icon="Lock" show-password class="input-ember" />
          </el-form-item>
          <el-form-item>
            <el-button native-type="submit" :loading="loading" class="btn-ember w-full">
              登录
            </el-button>
          </el-form-item>
        </el-form>

        <!-- Footer Link -->
        <div class="mt-6 text-center">
          <router-link to="/" class="text-sm text-text-secondary hover:text-ember transition-colors">
            ← 返回首页
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>
