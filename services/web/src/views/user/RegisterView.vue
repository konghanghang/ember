<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'
import { getRegistrationMode, validateRegistrationCode } from '@/api/auth'

const router = useRouter()
const authStore = useAuthStore()

const mode = ref<'open' | 'invite'>('open')
const loadingMode = ref(false)
const loading = ref(false)

const form = ref({
  username: '',
  password: '',
  email: '',
  code: ''
})

const codeRequired = computed(() => mode.value === 'invite')
const codeValidating = ref(false)
const codeValidated = ref(false)

const fetchRegistrationMode = async () => {
  loadingMode.value = true
  try {
    const res = await getRegistrationMode()
    mode.value = res.mode
  } finally {
    loadingMode.value = false
  }
}

const handleRegister = async () => {
  if (!form.value.username || !form.value.password || !form.value.email) {
    ElMessage.warning('请填写必填项')
    return
  }
  if (codeRequired.value && !form.value.code) {
    ElMessage.warning('当前为邀请注册模式，请输入兑换码')
    return
  }

  if (codeRequired.value && form.value.code && !codeValidated.value) {
    const ok = await handleValidateCode()
    if (!ok) return
  }

  loading.value = true
  try {
    await authStore.register({
      username: form.value.username,
      password: form.value.password,
      email: form.value.email,
      code: form.value.code || undefined
    })
    ElMessage.success('注册成功')
    router.push('/console/dashboard')
  } finally {
    loading.value = false
  }
}

const handleValidateCode = async () => {
  if (!form.value.code) return false
  codeValidating.value = true
  try {
    await validateRegistrationCode(form.value.code)
    codeValidated.value = true
    ElMessage.success('兑换码可用')
    return true
  } catch {
    codeValidated.value = false
    return false
  } finally {
    codeValidating.value = false
  }
}

onMounted(fetchRegistrationMode)
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-cinema-bg px-4">
    <div class="w-full max-w-md">
      <div class="panel-clean rounded-2xl p-8">
        <div class="text-center mb-8">
          <h1 class="text-3xl font-bold text-ember">Ember</h1>
          <p class="text-text-secondary mt-2">用户注册</p>
        </div>

        <el-form :model="form" @submit.prevent="handleRegister" size="large" label-position="top" v-loading="loadingMode">
          <el-form-item v-if="codeRequired" label="兑换码" required>
            <div class="flex gap-2 w-full">
              <el-input v-model="form.code" placeholder="请输入兑换码" class="input-ember" @input="codeValidated = false" />
              <el-button :loading="codeValidating" @click="handleValidateCode">预验证</el-button>
            </div>
          </el-form-item>

          <el-form-item label="用户名" required>
            <el-input v-model="form.username" placeholder="3-50位字符" class="input-ember" />
          </el-form-item>

          <el-form-item label="密码" required>
            <el-input v-model="form.password" type="password" placeholder="至少6位" show-password class="input-ember" />
          </el-form-item>

          <el-form-item label="邮箱" required>
            <el-input v-model="form.email" placeholder="请输入邮箱" class="input-ember" />
          </el-form-item>

          <el-form-item class="mt-6">
            <el-button native-type="submit" :loading="loading" class="btn-ember w-full">注册</el-button>
          </el-form-item>
        </el-form>

        <div class="mt-6 text-center">
          <router-link to="/" class="text-sm text-text-secondary hover:text-ember transition-colors">← 返回首页</router-link>
        </div>
      </div>
    </div>
  </div>
</template>
