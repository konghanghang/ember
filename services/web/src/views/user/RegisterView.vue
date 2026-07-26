<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Lock, Message, Ticket, User, ArrowLeft } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { getRegistrationMode, sendEmailCode, validateRegistrationCode } from '@/api/auth'

const router = useRouter()
const authStore = useAuthStore()

const mode = ref<'open' | 'invite'>('open')
const loadingMode = ref(false)
const loading = ref(false)
const emailVerification = ref(false)
const emailCodeCountdown = ref(0)
const sendingCode = ref(false)
const allowedEmailDomains = ref<string[]>([])
const emailDomainError = ref('')
let countdownTimer: ReturnType<typeof setInterval> | null = null

const form = ref({
  username: '',
  password: '',
  email: '',
  emailCode: '',
  code: ''
})

const codeRequired = computed(() => mode.value === 'invite')
const codeValidating = ref(false)
const codeValidated = ref(false)
const usernamePattern = /^[A-Za-z0-9]+$/

const hasDomainAllowlist = computed(() => allowedEmailDomains.value.length > 0)

const allowedDomainsHint = computed(() => {
  const domains = allowedEmailDomains.value
  if (domains.length === 0) return ''
  if (domains.length <= 3) {
    return `仅支持以下邮箱注册：${domains.join('、')}`
  }
  const head = domains.slice(0, 2).join('、')
  return `仅支持以下邮箱注册：${head} 等 ${domains.length} 个域名`
})

const extractEmailDomain = (email: string): string => {
  const trimmed = email.trim()
  const atIndex = trimmed.lastIndexOf('@')
  if (atIndex <= 0 || atIndex === trimmed.length - 1) return ''
  return trimmed.slice(atIndex + 1).toLowerCase()
}

const isEmailDomainAllowed = (email: string): boolean => {
  if (!hasDomainAllowlist.value) return true
  const domain = extractEmailDomain(email)
  if (!domain) return true
  return allowedEmailDomains.value.includes(domain)
}

const handleEmailBlur = () => {
  if (!hasDomainAllowlist.value) {
    emailDomainError.value = ''
    return
  }
  const email = form.value.email.trim()
  if (!email) {
    emailDomainError.value = ''
    return
  }
  if (!isEmailDomainAllowed(email)) {
    emailDomainError.value = '该邮箱域名不允许注册'
  } else {
    emailDomainError.value = ''
  }
}

const handleEmailInput = () => {
  if (emailDomainError.value) {
    emailDomainError.value = ''
  }
}

const resetCodeValidationState = () => {
  codeValidated.value = false
}

const fetchRegistrationMode = async () => {
  loadingMode.value = true
  try {
    const res = await getRegistrationMode()
    mode.value = res.mode
    emailVerification.value = res.emailVerification ?? false
    allowedEmailDomains.value = (res.allowedEmailDomains ?? []).map(d => d.trim().toLowerCase()).filter(Boolean)
  } finally {
    loadingMode.value = false
  }
}

const handleSendCode = async () => {
  if (!form.value.email) {
    ElMessage.warning('请先输入邮箱')
    return
  }

  sendingCode.value = true
  try {
    await sendEmailCode(form.value.email)
    ElMessage.success('验证码已发送，请查收邮件')

    if (countdownTimer) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }

    emailCodeCountdown.value = 60
    countdownTimer = setInterval(() => {
      emailCodeCountdown.value -= 1
      if (emailCodeCountdown.value <= 0) {
        emailCodeCountdown.value = 0
        if (countdownTimer) {
          clearInterval(countdownTimer)
          countdownTimer = null
        }
      }
    }, 1000)
  } finally {
    sendingCode.value = false
  }
}

const handleRegister = async () => {
  form.value.username = form.value.username.trim()

  if (!form.value.username || !form.value.password || !form.value.email) {
    ElMessage.warning('请填写必填项')
    return
  }
  if (!usernamePattern.test(form.value.username)) {
    ElMessage.warning('用户名只能包含字母和数字')
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
  if (emailVerification.value && !form.value.emailCode) {
    ElMessage.warning('请输入邮箱验证码')
    return
  }

  loading.value = true
  try {
    await authStore.register({
      username: form.value.username,
      password: form.value.password,
      email: form.value.email,
      emailCode: emailVerification.value ? form.value.emailCode : undefined,
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

onBeforeUnmount(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
})
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-cinema-bg p-4 relative overflow-hidden">
    <div class="absolute -top-[30%] -right-[10%] w-[70%] h-[70%] bg-ember/5 opacity-60 blur-[100px] rounded-full pointer-events-none"></div>
    <div class="absolute bottom-[10%] -left-[10%] w-[40%] h-[40%] bg-gray-50 opacity-60 blur-[80px] rounded-full pointer-events-none"></div>

    <div class="w-full max-w-md relative z-10 animate-fade-in">
      <router-link to="/" class="inline-flex items-center text-text-secondary hover:text-ember mb-8 transition-colors text-sm group">
        <el-icon class="mr-1 transition-transform group-hover:-translate-x-1"><ArrowLeft /></el-icon>
        返回首页
      </router-link>

      <div class="panel-clean rounded-2xl p-8 md:p-10" v-loading="loadingMode">
        <div class="text-center mb-10">
          <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-ember/10 text-ember mb-4">
            <el-icon class="text-2xl"><User /></el-icon>
          </div>
          <h1 class="text-2xl font-bold text-text-primary tracking-tight mb-2">用户注册</h1>
          <p class="text-text-secondary text-sm">创建您的 Ember 账号</p>
        </div>

        <el-form :model="form" @submit.prevent="handleRegister" size="large" class="space-y-6">
          <div class="space-y-4">
            <div v-if="codeRequired" class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">兑换码</label>
              <div class="flex gap-2 w-full">
                <el-input
                  v-model="form.code"
                  placeholder="请输入兑换码"
                  class="input-ember"
                  :prefix-icon="Ticket"
                  @input="resetCodeValidationState"
                />
                <el-button
                  type="button"
                  :loading="codeValidating"
                  class="!h-[42px] !rounded-xl shrink-0"
                  @click="handleValidateCode"
                >预验证</el-button>
              </div>
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">用户名</label>
              <el-input v-model="form.username" placeholder="3-50位字母或数字" class="input-ember" :prefix-icon="User" />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">密码</label>
              <el-input
                v-model="form.password"
                type="password"
                placeholder="至少6位"
                show-password
                class="input-ember"
                :prefix-icon="Lock"
              />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">邮箱</label>
              <div class="w-full flex flex-col gap-1.5">
                <div v-if="emailVerification" class="flex gap-2 w-full">
                  <el-input
                    v-model="form.email"
                    type="email"
                    placeholder="请输入邮箱"
                    class="input-ember"
                    :prefix-icon="Message"
                    @blur="handleEmailBlur"
                    @input="handleEmailInput"
                  />
                  <el-button
                    type="button"
                    :loading="sendingCode"
                    :disabled="emailCodeCountdown > 0"
                    class="!h-[42px] !rounded-xl shrink-0"
                    @click="handleSendCode"
                  >
                    {{ emailCodeCountdown > 0 ? `${emailCodeCountdown}s` : '发送验证码' }}
                  </el-button>
                </div>
                <el-input
                  v-else
                  v-model="form.email"
                  type="email"
                  placeholder="请输入邮箱"
                  class="input-ember"
                  :prefix-icon="Message"
                  @blur="handleEmailBlur"
                  @input="handleEmailInput"
                />
                <p v-if="emailDomainError" class="text-xs text-red-600 leading-5">
                  {{ emailDomainError }}
                </p>
                <p v-else-if="hasDomainAllowlist" class="text-xs text-gray-500 leading-5">
                  {{ allowedDomainsHint }}
                </p>
              </div>
            </div>

            <div v-if="emailVerification" class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">邮箱验证码</label>
              <el-input v-model="form.emailCode" placeholder="请输入 6 位验证码" maxlength="6" class="input-ember" />
            </div>
          </div>

          <el-button
            native-type="submit"
            :loading="loading"
            class="btn-ember w-full !h-12 !text-base !rounded-xl !font-semibold mt-2 shadow-lg"
          >
            注册
          </el-button>
        </el-form>

        <div class="mt-8 pt-6 border-t border-gray-100">
          <div class="flex items-center justify-center gap-6 text-sm">
            <router-link to="/login" class="text-text-secondary hover:text-ember transition-colors font-medium">
              已有账号？立即登录
            </router-link>
          </div>
        </div>
      </div>

      <p class="text-center text-text-muted text-xs mt-8">
        &copy; 2026 Ember Project
      </p>
    </div>
  </div>
</template>
