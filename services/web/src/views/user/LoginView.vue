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
    await authStore.userLogin(form.value)
    ElMessage.success('登录成功')
    router.push('/user/dashboard')
  } catch (error) {
    // Error handled in interceptor
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-orange-50 to-amber-100 dark:from-gray-900 dark:to-gray-800 px-4">
    <div class="w-full max-w-md">
      <div class="bg-white dark:bg-gray-800 rounded-2xl shadow-xl p-8">
        <!-- Logo -->
        <div class="text-center mb-8">
          <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
            Ember
          </h1>
          <p class="text-gray-600 dark:text-gray-400 mt-2">
            用户登录
          </p>
        </div>

        <!-- Login Form -->
        <el-form :model="form" @submit.prevent="handleLogin" size="large">
          <el-form-item>
            <el-input v-model="form.username" placeholder="请输入用户名" prefix-icon="User" />
          </el-form-item>
          <el-form-item>
            <el-input v-model="form.password" type="password" placeholder="请输入密码" prefix-icon="Lock" show-password />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" native-type="submit" :loading="loading" class="w-full bg-orange-600 hover:bg-orange-700 border-none">
              登录
            </el-button>
          </el-form-item>
        </el-form>

        <!-- Footer Links -->
        <div class="mt-6 text-center space-y-2">
          <div>
            <span class="text-sm text-gray-600 dark:text-gray-400">还没有账号？</span>
            <router-link to="/register" class="text-sm text-orange-600 dark:text-orange-400 hover:underline ml-1 font-medium">
              立即注册
            </router-link>
          </div>
          <div>
            <router-link to="/login" class="text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors">
              管理员登录 →
            </router-link>
          </div>
          <div>
            <router-link to="/" class="text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors">
              ← 返回首页
            </router-link>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
