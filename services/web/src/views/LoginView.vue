<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore } from '@/store/auth'
import { User, Lock, ArrowLeft } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const form = ref({
  username: '',
  password: ''
})
const loading = ref(false)
const redirectTarget = ref('/console/dashboard')
const turnstileToken = ref('')
const turnstileReady = ref(false)
const turnstileError = ref('')
const configLoading = ref(true)
const configError = ref<string | null>(null)
const turnstileWidgetRef = ref<{ reset: () => void } | null>(null)

const resolveSafeRedirect = (value: unknown) => {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) {
    return '/console/dashboard'
  }

  const resolved = router.resolve(value)
  if (resolved.matched.length === 0 || resolved.name === 'not-found') {
    return '/console/dashboard'
  }

  return value
}

const updateRedirectTarget = () => {
  redirectTarget.value = resolveSafeRedirect(route.query.redirect)
}

const isTurnstileEnabled = computed(() => {
  const config = authStore.protectionConfig
  return Boolean(config?.turnstileLoginEnabled && config.turnstileSiteKey?.trim())
})

const turnstileSiteKey = computed(() => authStore.protectionConfig?.turnstileSiteKey?.trim() ?? '')

const turnstileHint = computed(() => {
  if (!isTurnstileEnabled.value) {
    return ''
  }
  if (turnstileError.value) {
    return turnstileError.value
  }
  if (!turnstileReady.value) {
    return 'Turnstile 验证组件加载中...'
  }
  if (!turnstileToken.value) {
    return '请完成 Turnstile 验证后再登录'
  }
  return '验证通过，可继续登录'
})

const turnstileHintClass = computed(() => (turnstileError.value ? 'text-ember' : 'text-text-secondary'))
const loginBlockedReason = computed(() => {
  if (configLoading.value) {
    return '登录保护配置加载中，请稍后再试'
  }
  if (configError.value) {
    return configError.value
  }
  return ''
})

const resetTurnstileState = () => {
  turnstileToken.value = ''
  turnstileReady.value = false
  turnstileError.value = ''
}

const handleTurnstileReady = () => {
  turnstileReady.value = true
  turnstileError.value = ''
}

const handleTurnstileResolved = (token: string) => {
  turnstileToken.value = token
  turnstileError.value = ''
}

const handleTurnstileError = (message: string) => {
  turnstileError.value = message
  turnstileToken.value = ''
  turnstileReady.value = false
}

const loadProtectionConfig = async () => {
  configLoading.value = true
  configError.value = null
  try {
    const config = await authStore.loadProtectionConfig()
    if (config.turnstileLoginEnabled && !config.turnstileSiteKey?.trim()) {
      configError.value = '登录保护未完成配置，暂时关闭验证'
    }
  } catch {
    configError.value = '登录保护配置加载失败，刷新页面重试'
  } finally {
    configLoading.value = false
  }
}

updateRedirectTarget()

onMounted(() => {
  void loadProtectionConfig()
})

watch(() => route.query.redirect, updateRedirectTarget)
watch(turnstileSiteKey, (current, previous) => {
  if (current !== previous) {
    resetTurnstileState()
    turnstileWidgetRef.value?.reset()
  }
})
watch(isTurnstileEnabled, (enabled) => {
  if (!enabled) {
    resetTurnstileState()
    turnstileWidgetRef.value?.reset()
  }
})

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }

  if (loginBlockedReason.value) {
    ElMessage.warning(loginBlockedReason.value)
    return
  }

  if (isTurnstileEnabled.value && !turnstileToken.value) {
    ElMessage.warning('请完成 Turnstile 验证')
    return
  }

  loading.value = true
  try {
    const payload = {
      ...form.value,
      ...(turnstileToken.value ? { turnstileToken: turnstileToken.value } : {})
    }
    const result = await authStore.login(payload)
    ElMessage.success('登录成功')
    if (result.user.passwordResetRequired) {
      ElMessage.warning('当前账号必须先修改密码')
      await router.replace({ name: 'console-account' })
      return
    }
    await router.replace(redirectTarget.value)
  } catch {
    if (isTurnstileEnabled.value) {
      resetTurnstileState()
      turnstileWidgetRef.value?.reset()
    }
    // 错误提示由全局请求拦截器统一处理，避免重复弹窗
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-cinema-bg flex items-center justify-center p-4 relative overflow-hidden">
    <div class="absolute -top-[30%] -right-[10%] w-[70%] h-[70%] bg-ember/5 opacity-60 blur-[100px] rounded-full pointer-events-none"></div>
    <div class="absolute bottom-[10%] -left-[10%] w-[40%] h-[40%] bg-gray-50 opacity-60 blur-[80px] rounded-full pointer-events-none"></div>

    <div class="w-full max-w-md relative z-10 animate-fade-in">

      <router-link to="/" class="inline-flex items-center text-text-secondary hover:text-ember mb-8 transition-colors text-sm group">
        <el-icon class="mr-1 transition-transform group-hover:-translate-x-1"><ArrowLeft /></el-icon>
        返回首页
      </router-link>

      <div class="panel-clean rounded-2xl p-8 md:p-10">

        <div class="text-center mb-10">
          <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-ember/10 text-ember mb-4">
            <el-icon class="text-2xl"><User /></el-icon>
          </div>
          <h1 class="text-2xl font-bold text-text-primary tracking-tight mb-2">欢迎回来</h1>
          <p class="text-text-secondary text-sm">登录您的 Ember 账号</p>
        </div>

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

          <div class="space-y-2">
            <div v-if="configLoading" class="text-center text-xs text-text-secondary">
              登录保护配置加载中...
            </div>
            <div v-else>
              <div v-if="configError" class="text-center text-xs font-medium text-ember">
                {{ configError }}
              </div>
              <div v-else-if="isTurnstileEnabled" class="space-y-2">
                <div class="turnstile-shell">
                  <TurnstileWidget
                    v-if="turnstileSiteKey"
                    ref="turnstileWidgetRef"
                    :siteKey="turnstileSiteKey"
                    size="flexible"
                    @ready="handleTurnstileReady"
                    @resolved="handleTurnstileResolved"
                    @error="handleTurnstileError"
                  />
                </div>
                <p class="text-xs" :class="turnstileHintClass">{{ turnstileHint }}</p>
              </div>
            </div>
          </div>

          <el-button
            native-type="submit"
            :loading="loading"
            :disabled="Boolean(loginBlockedReason)"
            class="btn-ember w-full !h-12 !text-base !rounded-xl !font-semibold mt-2 shadow-lg"
          >
            登 录
          </el-button>
        </el-form>

        <div class="mt-8 pt-6 border-t border-gray-100">
          <div class="flex items-center justify-center gap-6 text-sm">
            <router-link to="/forgot-password" class="text-text-secondary hover:text-ember transition-colors font-medium">
              忘记密码？
            </router-link>
            <span class="text-gray-200">|</span>
            <router-link to="/register" class="text-text-secondary hover:text-ember transition-colors font-medium">
              注册新账号
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

<style scoped>
.turnstile-shell {
  width: 100%;
  min-height: 76px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0.75rem;
  border-radius: 0.75rem;
  background-color: #f9fafb;
  box-shadow: 0 0 0 1px #e5e7eb inset;
}
</style>
