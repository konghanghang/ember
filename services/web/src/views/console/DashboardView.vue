<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { 
  UserFilled, 
  Key, 
  Message, 
  Ticket, 
  Lock, 
  CircleCloseFilled, 
  VideoPlay, 
  Monitor, 
  Film,
  CopyDocument,
  ArrowRight
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { getEmbyConfig, getMediaStats, getProfile, updateEmail, updatePassword } from '@/api/console'
import { redeemCode } from '@/api/user'
import type { MediaStats, UserInfo } from '@/types/api'

const authStore = useAuthStore()

const user = ref<UserInfo>({
  id: '',
  username: '',
  role: 'user',
  email: '',
  embyId: '',
  expiresAt: '',
  embyDisabled: false,
  isActive: false,
  createdAt: ''
})

const embyUrl = ref('')
const stats = ref<MediaStats>({ MovieCount: 0, SeriesCount: 0, EpisodeCount: 0 })
const loading = ref(false)
const redeeming = ref(false)
const redeemForm = ref({ code: '' })
const showRenewDialog = ref(false)

const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const isExpired = computed(() => {
  if (!user.value.expiresAt) return false
  return new Date(user.value.expiresAt) < new Date()
})

const daysLeft = computed(() => {
  if (!user.value.expiresAt) return 0
  const ms = new Date(user.value.expiresAt).getTime() - Date.now()
  return Math.ceil(ms / (24 * 60 * 60 * 1000))
})

const fetchProfile = async () => {
  loading.value = true
  try {
    user.value = await getProfile()
  } finally {
    loading.value = false
  }
}

const fetchMediaInfo = async () => {
  if (!authStore.isAdmin && isExpired.value) return
  const [configRes, statsRes] = await Promise.all([getEmbyConfig(), getMediaStats()])
  if (configRes.success) embyUrl.value = configRes.url
  if (statsRes.success) stats.value = statsRes.data
}

const refreshAll = async () => {
  await fetchProfile()
  await fetchMediaInfo()
}

const handleRedeem = async () => {
  if (!redeemForm.value.code) {
    ElMessage.warning('请输入兑换码')
    return
  }

  redeeming.value = true
  try {
    const res = await redeemCode({ code: redeemForm.value.code })
    ElMessage.success(res.message)
    redeemForm.value.code = ''
    showRenewDialog.value = false
    await refreshAll()
  } catch {
    // handled
  } finally {
    redeeming.value = false
  }
}

const handleUpdateEmail = async () => {
  try {
    await updateEmail(user.value.email || '')
    ElMessage.success('邮箱更新成功')
  } catch {
    // handled
  }
}

const handleUpdatePassword = async () => {
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    ElMessage.warning('两次输入密码不一致')
    return
  }

  try {
    await updatePassword({
      oldPassword: passwordForm.value.oldPassword,
      newPassword: passwordForm.value.newPassword
    })
    ElMessage.success('密码修改成功')
    passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  } catch {
    // handled
  }
}

const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('复制成功')
  } catch {
    ElMessage.error('复制失败')
  }
}

onMounted(refreshAll)
</script>

<template>
  <div class="space-y-8 animate-fade-in" v-loading="loading">
    <!-- Hero Membership Card -->
    <div class="relative overflow-hidden rounded-3xl bg-gray-900 text-white shadow-xl">
      <!-- Background Gradients -->
      <div class="absolute top-0 right-0 -mr-20 -mt-20 w-96 h-96 bg-ember rounded-full mix-blend-multiply filter blur-3xl opacity-20 animate-blob"></div>
      <div class="absolute bottom-0 left-0 -ml-20 -mb-20 w-96 h-96 bg-purple-600 rounded-full mix-blend-multiply filter blur-3xl opacity-20 animate-blob animation-delay-2000"></div>
      
      <div class="relative p-8 md:p-10">
        <div class="flex flex-col md:flex-row justify-between items-start md:items-center gap-6">
          <!-- User Profile -->
          <div class="flex items-center gap-6">
            <div class="w-20 h-20 rounded-full bg-gradient-to-br from-gray-700 to-gray-800 border-4 border-gray-800 flex items-center justify-center shadow-lg">
              <span class="text-3xl font-bold text-gray-400">
                {{ user.username.charAt(0).toUpperCase() }}
              </span>
            </div>
            <div>
              <div class="flex items-center gap-3">
                <h1 class="text-3xl font-bold tracking-tight">{{ user.username }}</h1>
                <span 
                  class="px-3 py-1 rounded-full text-xs font-bold uppercase tracking-wider border"
                  :class="authStore.isAdmin ? 'bg-blue-500/20 border-blue-500/50 text-blue-300' : 'bg-ember/20 border-ember/50 text-red-300'"
                >
                  {{ authStore.isAdmin ? '管理员' : '高级会员' }}
                </span>
              </div>
              <p class="text-gray-400 mt-2 font-mono text-sm flex items-center gap-2">
                ID: {{ user.embyId || '待激活' }}
                <button v-if="user.embyId" @click="copyToClipboard(user.embyId)" class="hover:text-white transition-colors">
                  <el-icon><CopyDocument /></el-icon>
                </button>
              </p>
            </div>
          </div>

          <!-- Status & Actions -->
          <div class="flex flex-col items-end gap-4">
            <div class="text-right">
              <p class="text-sm text-gray-400 mb-1">订阅状态</p>
              <div class="flex items-center gap-2 justify-end">
                <div class="w-2 h-2 rounded-full" :class="isExpired ? 'bg-red-500' : 'bg-green-500 animate-pulse'"></div>
                <span class="text-xl font-bold" :class="isExpired ? 'text-red-400' : 'text-green-400'">
                  {{ isExpired ? '已过期' : '有效' }}
                </span>
              </div>
            </div>
            
            <button 
              v-if="!authStore.isAdmin"
              @click="showRenewDialog = true"
              class="group flex items-center gap-2 px-6 py-2.5 bg-white text-gray-900 rounded-xl font-bold hover:bg-gray-100 transition-all shadow-lg active:scale-95"
            >
              <el-icon><Ticket /></el-icon>
              <span>{{ isExpired ? '立即续期' : '延长订阅' }}</span>
              <el-icon class="group-hover:translate-x-1 transition-transform"><ArrowRight /></el-icon>
            </button>
          </div>
        </div>

        <!-- Progress Bar (Visual Flair) -->
        <div class="mt-10">
          <div class="flex justify-between text-xs text-gray-500 mb-2 font-medium uppercase tracking-wider">
            <span>会员有效期进度</span>
            <span>{{ isExpired ? '剩余 0 天' : `剩余 ${daysLeft} 天` }}</span>
          </div>
          <div class="h-2 bg-gray-800 rounded-full overflow-hidden">
            <div 
              class="h-full bg-gradient-to-r from-ember to-orange-500 rounded-full transition-all duration-1000 ease-out"
              :style="{ width: isExpired ? '0%' : '100%' }"
            ></div>
          </div>
          <div class="mt-2 text-right text-xs text-gray-600">
            有效期至 {{ user.expiresAt ? new Date(user.expiresAt).toLocaleDateString() : '永久有效' }}
          </div>
        </div>
      </div>
    </div>

    <!-- Alert for Expired Users -->
    <div v-if="isExpired && !authStore.isAdmin" class="bg-red-50 border border-red-100 rounded-2xl p-4 flex items-center gap-4 text-red-800 animate-bounce-in">
      <div class="w-10 h-10 bg-red-100 rounded-full flex items-center justify-center flex-shrink-0">
        <el-icon :size="20"><CircleCloseFilled /></el-icon>
      </div>
      <div class="flex-1">
        <h3 class="font-bold text-sm">服务已暂停</h3>
        <p class="text-xs mt-1 text-red-600">您的订阅已过期。请续期以恢复 Emby 服务器访问权限。</p>
      </div>
      <button @click="showRenewDialog = true" class="text-sm font-bold underline hover:text-red-900">立即续期</button>
    </div>

    <!-- Stats Row -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm flex items-center gap-4 hover:border-purple-200 transition-colors group">
        <div class="w-14 h-14 bg-purple-50 rounded-xl flex items-center justify-center text-purple-500 group-hover:scale-110 transition-transform duration-300">
          <el-icon :size="28"><Film /></el-icon>
        </div>
        <div>
          <p class="text-3xl font-bold text-gray-900">{{ stats.MovieCount }}</p>
          <p class="text-sm text-gray-500 font-medium">电影收藏</p>
        </div>
      </div>
      
      <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm flex items-center gap-4 hover:border-green-200 transition-colors group">
        <div class="w-14 h-14 bg-green-50 rounded-xl flex items-center justify-center text-green-500 group-hover:scale-110 transition-transform duration-300">
          <el-icon :size="28"><VideoPlay /></el-icon>
        </div>
        <div>
          <p class="text-3xl font-bold text-gray-900">{{ stats.SeriesCount }}</p>
          <p class="text-sm text-gray-500 font-medium">剧集收藏</p>
        </div>
      </div>

      <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm flex items-center gap-4 hover:border-blue-200 transition-colors group">
        <div class="w-14 h-14 bg-blue-50 rounded-xl flex items-center justify-center text-blue-500 group-hover:scale-110 transition-transform duration-300">
          <el-icon :size="28"><Monitor /></el-icon>
        </div>
        <div>
          <p class="text-3xl font-bold text-gray-900">{{ stats.EpisodeCount }}</p>
          <p class="text-sm text-gray-500 font-medium">总集数</p>
        </div>
      </div>
    </div>

    <!-- Server & Settings Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
      
      <!-- Emby Server Info -->
      <div class="lg:col-span-1 space-y-6">
        <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden h-full">
          <div class="p-6 border-b border-gray-50 bg-gray-50/50">
            <h3 class="font-bold text-gray-900 flex items-center gap-2">
              <el-icon class="text-ember"><Monitor /></el-icon>
              服务器连接
            </h3>
          </div>
          <div class="p-6">
            <div v-if="embyUrl && (!isExpired || authStore.isAdmin)" class="space-y-4">
              <div class="bg-gray-50 p-4 rounded-xl border border-gray-100">
                <p class="text-xs text-gray-500 uppercase tracking-wider font-bold mb-2">服务器地址</p>
                <div class="flex items-center gap-2">
                  <code class="flex-1 bg-white px-3 py-2 rounded border border-gray-200 text-sm font-mono text-gray-700 truncate select-all">
                    {{ embyUrl }}
                  </code>
                  <button @click="copyToClipboard(embyUrl)" class="p-2 text-gray-400 hover:text-ember transition-colors">
                    <el-icon><CopyDocument /></el-icon>
                  </button>
                </div>
              </div>
              <p class="text-xs text-gray-500 leading-relaxed">
                使用此地址在 Emby 客户端登录。账号密码与本控制台一致。
              </p>
            </div>
            <div v-else class="text-center py-8 text-gray-400">
              <el-icon :size="48" class="mb-3 text-gray-300"><Lock /></el-icon>
              <p class="text-sm">服务器访问已锁定。</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Account Management -->
      <div class="lg:col-span-2 space-y-6">
        <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
          <div class="flex border-b border-gray-100">
            <div class="px-6 py-4 border-b-2 border-ember text-ember font-bold text-sm">账号设置</div>
            <div class="px-6 py-4 text-gray-500 font-medium text-sm hover:text-gray-900 cursor-not-allowed opacity-50">偏好设置</div>
          </div>
          
          <div class="p-8 grid grid-cols-1 md:grid-cols-2 gap-10">
            <!-- Email Update -->
            <div class="space-y-4">
              <h4 class="font-bold text-gray-900 text-sm flex items-center gap-2">
                <el-icon class="text-gray-400"><Message /></el-icon>
                联系邮箱
              </h4>
              <div class="flex gap-3">
                <el-input
                  v-model="user.email"
                  placeholder="new@email.com"
                  class="input-ember"
                  :prefix-icon="Message"
                />
                <button 
                  @click="handleUpdateEmail"
                  class="px-4 py-2 bg-gray-900 text-white rounded-lg hover:bg-black transition-colors text-sm font-bold"
                >
                  保存
                </button>
              </div>
              <p class="text-xs text-gray-400">用于找回密码和接收系统通知。</p>
            </div>

            <!-- Password Update -->
            <div class="space-y-4">
              <h4 class="font-bold text-gray-900 text-sm flex items-center gap-2">
                <el-icon class="text-gray-400"><Key /></el-icon>
                修改密码
              </h4>
              <el-input 
                v-model="passwordForm.oldPassword" 
                type="password" 
                show-password 
                placeholder="当前密码" 
                class="input-ember"
                :prefix-icon="Lock"
              />
              <el-input 
                v-model="passwordForm.newPassword" 
                type="password" 
                show-password 
                placeholder="新密码" 
                class="input-ember"
                :prefix-icon="Lock"
              />
              <el-input 
                v-model="passwordForm.confirmPassword" 
                type="password" 
                show-password 
                placeholder="确认新密码" 
                class="input-ember"
                :prefix-icon="Lock"
              />
              <button 
                @click="handleUpdatePassword"
                class="w-full py-2.5 bg-ember text-white rounded-lg hover:bg-red-700 transition-colors font-bold shadow-md hover:shadow-lg active:scale-95 text-sm"
              >
                更新密码
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Renew Dialog -->
    <el-dialog v-model="showRenewDialog" title="续期会员" width="400px" align-center class="rounded-2xl">
      <div class="p-6 pt-2 text-center">
        <div class="w-16 h-16 bg-red-50 rounded-full flex items-center justify-center mx-auto mb-4 text-ember">
          <el-icon :size="32"><Ticket /></el-icon>
        </div>
        <h3 class="text-lg font-bold text-gray-900 mb-2">输入兑换码</h3>
        <p class="text-gray-500 text-sm mb-6">在下方输入您的兑换码以立即延长订阅。</p>
        
        <el-input 
          v-model="redeemForm.code" 
          placeholder="在此输入兑换码..." 
          class="input-ember text-center text-lg mb-6"
          size="large"
        />
        
        <button 
          @click="handleRedeem" 
          :disabled="redeeming"
          class="w-full py-3 bg-ember text-white rounded-xl font-bold hover:bg-red-700 transition-colors shadow-lg hover:shadow-xl disabled:opacity-70 flex items-center justify-center gap-2"
        >
          <span v-if="redeeming" class="animate-spin w-4 h-4 border-2 border-white/30 border-t-white rounded-full"></span>
          {{ redeeming ? '验证中...' : '确认兑换' }}
        </button>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.6s ease-out forwards;
}

.animate-blob {
  animation: blob 7s infinite;
}

.animation-delay-2000 {
  animation-delay: 2s;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes blob {
  0% { transform: translate(0px, 0px) scale(1); }
  33% { transform: translate(30px, -50px) scale(1.1); }
  66% { transform: translate(-20px, 20px) scale(0.9); }
  100% { transform: translate(0px, 0px) scale(1); }
}

:deep(.el-input__wrapper) {
  background-color: #f9fafb;
  box-shadow: 0 0 0 1px #e5e7eb inset;
}
:deep(.el-input__wrapper.is-focus) {
  background-color: white;
  box-shadow: 0 0 0 2px var(--ember-red) inset !important;
}
</style>
