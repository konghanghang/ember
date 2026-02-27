<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Lock, Message } from '@element-plus/icons-vue'
import { resetPasswordByCode, sendResetCode } from '@/api/auth'

const router = useRouter()

const step = ref(1)
const loading = ref(false)
const sendingCode = ref(false)
const emailCodeCountdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const form = ref({
  email: '',
  code: '',
  newPassword: '',
  confirmPassword: ''
})

const clearCountdown = () => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

const startCountdown = () => {
  clearCountdown()
  emailCodeCountdown.value = 60
  countdownTimer = setInterval(() => {
    emailCodeCountdown.value -= 1
    if (emailCodeCountdown.value <= 0) {
      emailCodeCountdown.value = 0
      clearCountdown()
    }
  }, 1000)
}

const handleSendCode = async () => {
  if (!form.value.email) {
    ElMessage.warning('请输入邮箱')
    return
  }

  sendingCode.value = true
  try {
    await sendResetCode(form.value.email)
    ElMessage.success('验证码已发送，请查收邮件')
    step.value = 2
    startCountdown()
  } catch {
    // 统一错误提示由请求拦截器处理，避免重复弹窗
  } finally {
    sendingCode.value = false
  }
}

const handleReset = async () => {
  if (!form.value.code || !form.value.newPassword || !form.value.confirmPassword) {
    ElMessage.warning('请填写完整信息')
    return
  }
  if (form.value.newPassword.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  if (form.value.newPassword !== form.value.confirmPassword) {
    ElMessage.warning('两次密码不一致')
    return
  }

  loading.value = true
  try {
    await resetPasswordByCode({
      email: form.value.email,
      code: form.value.code,
      newPassword: form.value.newPassword
    })
    ElMessage.success('密码重置成功，请重新登录')
    router.push('/login')
  } catch {
    // 统一错误提示由请求拦截器处理，避免重复弹窗
  } finally {
    loading.value = false
  }
}

onBeforeUnmount(() => {
  clearCountdown()
})
</script>

<template>
  <div class="min-h-screen bg-cinema-bg flex items-center justify-center p-4 relative overflow-hidden">
    <div class="absolute -top-[30%] -right-[10%] w-[70%] h-[70%] bg-ember/5 opacity-60 blur-[100px] rounded-full pointer-events-none"></div>
    <div class="absolute bottom-[10%] -left-[10%] w-[40%] h-[40%] bg-gray-50 opacity-60 blur-[80px] rounded-full pointer-events-none"></div>

    <div class="w-full max-w-md relative z-10 animate-fade-in">
      <router-link to="/login" class="inline-flex items-center text-text-secondary hover:text-ember mb-8 transition-colors text-sm group">
        <el-icon class="mr-1 transition-transform group-hover:-translate-x-1"><ArrowLeft /></el-icon>
        返回登录
      </router-link>

      <div class="panel-clean rounded-2xl p-8 md:p-10">
        <div class="text-center mb-10">
          <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-ember/10 text-ember mb-4">
            <el-icon class="text-2xl"><Lock /></el-icon>
          </div>
          <h1 class="text-2xl font-bold text-text-primary tracking-tight mb-2">重置密码</h1>
          <p class="text-text-secondary text-sm">
            {{ step === 1 ? '输入注册时的邮箱地址' : '输入验证码和新密码' }}
          </p>
        </div>

        <el-form v-if="step === 1" @submit.prevent="handleSendCode" size="large" class="space-y-6">
          <div class="space-y-1">
            <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">邮箱</label>
            <el-input
              v-model="form.email"
              placeholder="请输入注册邮箱"
              class="input-ember"
              :prefix-icon="Message"
            />
          </div>

          <el-button
            native-type="submit"
            :loading="sendingCode"
            class="btn-ember w-full !h-12 !text-base !rounded-xl !font-semibold mt-2 shadow-lg"
          >
            发送验证码
          </el-button>
        </el-form>

        <el-form v-else @submit.prevent="handleReset" size="large" class="space-y-6">
          <div class="space-y-4">
            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">邮箱</label>
              <el-input :model-value="form.email" disabled class="input-ember" :prefix-icon="Message" />
            </div>

            <div class="space-y-1">
              <div class="flex items-center justify-between ml-1 mr-1">
                <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider">验证码</label>
                <button
                  type="button"
                  class="text-xs text-ember hover:text-ember/80 transition-colors disabled:text-text-muted disabled:cursor-not-allowed"
                  :disabled="emailCodeCountdown > 0 || sendingCode"
                  @click="handleSendCode"
                >
                  {{ emailCodeCountdown > 0 ? `${emailCodeCountdown}s 后重发` : '重新发送' }}
                </button>
              </div>
              <el-input
                v-model="form.code"
                placeholder="请输入 6 位验证码"
                maxlength="6"
                class="input-ember"
              />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">新密码</label>
              <el-input
                v-model="form.newPassword"
                type="password"
                placeholder="至少 6 位"
                class="input-ember"
                :prefix-icon="Lock"
                show-password
              />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">确认密码</label>
              <el-input
                v-model="form.confirmPassword"
                type="password"
                placeholder="再次输入新密码"
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
            重置密码
          </el-button>
        </el-form>
      </div>

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
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
