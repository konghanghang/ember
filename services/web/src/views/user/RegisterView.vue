<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Lock, Message, Ticket, User } from '@element-plus/icons-vue'
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
              <el-input
                v-model="form.code"
                placeholder="请输入兑换码"
                class="input-ember"
                :prefix-icon="Ticket"
                @input="resetCodeValidationState"
              />
              <el-button :loading="codeValidating" @click="handleValidateCode">预验证</el-button>
            </div>
          </el-form-item>

          <el-form-item label="用户名" required>
            <el-input v-model="form.username" placeholder="3-50位字母或数字" class="input-ember" :prefix-icon="User" />
          </el-form-item>

          <el-form-item label="密码" required>
            <el-input
              v-model="form.password"
              type="password"
              placeholder="至少6位"
              show-password
              class="input-ember"
              :prefix-icon="Lock"
            />
          </el-form-item>

          <el-form-item label="邮箱" required>
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
                  native-type="button"
                  :loading="sendingCode"
                  :disabled="emailCodeCountdown > 0"
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
          </el-form-item>

          <el-form-item v-if="emailVerification" label="邮箱验证码" required>
            <el-input v-model="form.emailCode" placeholder="请输入 6 位验证码" maxlength="6" class="input-ember" />
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
