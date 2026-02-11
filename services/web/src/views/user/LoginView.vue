<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'
import { User, Lock, ArrowLeft } from '@element-plus/icons-vue'

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
  } catch {
    ElMessage.error('登录失败，请检查用户名或密码')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-cinema-bg flex items-center justify-center p-4 relative overflow-hidden">
    <!-- Clean Background Accent -->
    <div class="absolute -top-[30%] -right-[10%] w-[70%] h-[70%] bg-ember/5 opacity-60 blur-[100px] rounded-full pointer-events-none"></div>
    <div class="absolute bottom-[10%] -left-[10%] w-[40%] h-[40%] bg-gray-50 opacity-60 blur-[80px] rounded-full pointer-events-none"></div>

    <div class="w-full max-w-md relative z-10 animate-fade-in">
      
      <!-- Back Link -->
      <router-link to="/" class="inline-flex items-center text-text-secondary hover:text-ember mb-8 transition-colors text-sm group">
        <el-icon class="mr-1 transition-transform group-hover:-translate-x-1"><ArrowLeft /></el-icon>
        返回首页
      </router-link>

      <!-- Clean Card -->
      <div class="panel-clean rounded-2xl p-8 md:p-10">
        
        <!-- Header -->
        <div class="text-center mb-10">
          <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-ember/10 text-ember mb-4">
            <el-icon class="text-2xl"><User /></el-icon>
          </div>
          <h1 class="text-2xl font-bold text-text-primary tracking-tight mb-2">欢迎回来</h1>
          <p class="text-text-secondary text-sm">登录您的 Ember 私人账号</p>
        </div>

        <!-- Form -->
        <el-form :model="form" @submit.prevent="handleLogin" size="large" class="space-y-6">
          <div class="space-y-4">
            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">用户名</label>
              <el-input 
                v-model="form.username" 
                placeholder="请输入用户名" 
                class="input-ember"
                :prefix-icon="User" 
              />
            </div>
            
            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">密码</label>
              <el-input 
                v-model="form.password" 
                type="password" 
                placeholder="••••••••" 
                class="input-ember"
                :prefix-icon="Lock" 
                show-password 
              />
            </div>
          </div>

          <el-button 
            native-type="submit" 
            :loading="loading" 
            class="btn-ember w-full !h-12 !text-base !rounded-xl !font-semibold mt-2 shadow-lg"
          >
            登 录
          </el-button>
        </el-form>

        <!-- Footer -->
        <div class="mt-8 pt-6 border-t border-gray-100 flex items-center justify-between text-sm">
          <router-link to="/register" class="text-text-secondary hover:text-ember transition-colors font-medium">
            注册新账号
          </router-link>
          <router-link to="/login" class="text-text-muted hover:text-text-primary transition-colors">
            管理员登录
          </router-link>
        </div>

      </div>
      
      <!-- Footer Note -->
      <p class="text-center text-text-muted text-xs mt-8">
        &copy; 2026 Ember Project
      </p>

    </div>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.6s ease-out forwards;
}
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>