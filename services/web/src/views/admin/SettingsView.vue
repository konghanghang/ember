<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { 
  Calendar, 
  Connection, 
  RefreshRight, 
  Setting, 
  User, 
  Monitor, 
  Ticket,
  VideoPlay
} from '@element-plus/icons-vue'
import { getSystemInfo, testEmbyConnection, runCronJob, getSettings, updateSetting } from '@/api/admin'

const info = ref({
  userCount: 0,
  activeUserCount: 0,
  redemptionCodeCount: 0
})

const form = ref({
  registration_mode: 'open',
  default_trial_days: 7,
  notify_group_link: ''
})

const loading = ref(false)
const saving = ref(false)
const testingEmby = ref(false)
const runningCron = ref(false)

const fetchInfo = async () => {
  const res = await getSystemInfo()
  if (res.info) {
    info.value = res.info
  }
}

const fetchSettings = async () => {
  const list = await getSettings()
  const mode = list.find(item => item.key === 'registration_mode')
  const trial = list.find(item => item.key === 'default_trial_days')
  const notifyLink = list.find(item => item.key === 'notify_group_link')
  if (mode?.value) form.value.registration_mode = mode.value
  if (trial?.value) form.value.default_trial_days = Number(trial.value) || 7
  if (notifyLink?.value !== undefined) form.value.notify_group_link = notifyLink.value
}

const handleSaveSettings = async () => {
  saving.value = true
  try {
    await updateSetting('registration_mode', { value: form.value.registration_mode })
    await updateSetting('default_trial_days', { value: String(form.value.default_trial_days) })
    await updateSetting('notify_group_link', { value: form.value.notify_group_link })
    ElMessage.success('配置保存成功')
  } finally {
    saving.value = false
  }
}

const handleTestEmby = async () => {
  testingEmby.value = true
  try {
    const res = await testEmbyConnection()
    ElMessage.success(res.message || '连接正常')
  } catch {
    // handled
  } finally {
    testingEmby.value = false
  }
}

const handleRunCron = async () => {
  runningCron.value = true
  try {
    const res = await runCronJob()
    ElMessage.success((res as unknown as { message?: string }).message || '任务执行成功')
  } catch {
    // handled
  } finally {
    runningCron.value = false
  }
}

onMounted(async () => {
  loading.value = true
  try {
    await Promise.all([fetchInfo(), fetchSettings()])
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="space-y-8 animate-fade-in" v-loading="loading">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">系统设置</h1>
        <p class="text-gray-500 mt-1">管理系统全局参数与运行状态</p>
      </div>
      <div class="flex gap-3">
        <button 
          @click="fetchInfo" 
          class="p-2 text-gray-400 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
          title="刷新数据"
        >
          <el-icon :size="20"><RefreshRight /></el-icon>
        </button>
      </div>
    </div>

    <!-- Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="bg-white rounded-2xl p-6 border border-gray-100 shadow-sm flex items-center justify-between group hover:border-blue-100 hover:shadow-md transition-all">
        <div>
          <p class="text-sm font-medium text-gray-500 mb-1">总用户数</p>
          <p class="text-3xl font-bold text-gray-900 group-hover:text-blue-600 transition-colors">{{ info.userCount }}</p>
        </div>
        <div class="w-12 h-12 bg-blue-50 rounded-xl flex items-center justify-center text-blue-500">
          <el-icon :size="24"><User /></el-icon>
        </div>
      </div>

      <div class="bg-white rounded-2xl p-6 border border-gray-100 shadow-sm flex items-center justify-between group hover:border-green-100 hover:shadow-md transition-all">
        <div>
          <p class="text-sm font-medium text-gray-500 mb-1">活跃用户 (订阅中)</p>
          <p class="text-3xl font-bold text-gray-900 group-hover:text-green-600 transition-colors">{{ info.activeUserCount }}</p>
        </div>
        <div class="w-12 h-12 bg-green-50 rounded-xl flex items-center justify-center text-green-500">
          <el-icon :size="24"><VideoPlay /></el-icon>
        </div>
      </div>

      <div class="bg-white rounded-2xl p-6 border border-gray-100 shadow-sm flex items-center justify-between group hover:border-orange-100 hover:shadow-md transition-all">
        <div>
          <p class="text-sm font-medium text-gray-500 mb-1">兑换码总数</p>
          <p class="text-3xl font-bold text-gray-900 group-hover:text-orange-600 transition-colors">{{ info.redemptionCodeCount }}</p>
        </div>
        <div class="w-12 h-12 bg-orange-50 rounded-xl flex items-center justify-center text-orange-500">
          <el-icon :size="24"><Ticket /></el-icon>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
      <!-- Registration Settings -->
      <div class="lg:col-span-2 space-y-6">
        <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
          <div class="px-6 py-4 border-b border-gray-50 flex items-center gap-2 bg-gray-50/50">
            <el-icon class="text-gray-400"><Setting /></el-icon>
            <h3 class="font-bold text-gray-900">注册与试用配置</h3>
          </div>
          
          <div class="p-8">
            <el-form label-position="top" class="space-y-6">
              <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
                <el-form-item label="注册模式">
                  <div class="bg-gray-50 p-1 rounded-xl inline-flex w-full">
                    <button 
                      type="button"
                      @click="form.registration_mode = 'open'"
                      class="flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all"
                      :class="form.registration_mode === 'open' ? 'bg-white text-green-600 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
                    >
                      开放注册
                    </button>
                    <button 
                      type="button"
                      @click="form.registration_mode = 'invite'"
                      class="flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all"
                      :class="form.registration_mode === 'invite' ? 'bg-white text-blue-600 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
                    >
                      邀请码注册
                    </button>
                  </div>
                  <p class="text-xs text-gray-400 mt-2">
                    {{ form.registration_mode === 'open' ? '任何人都可以注册账号。' : '仅持有有效邀请码的用户可以注册。' }}
                  </p>
                </el-form-item>

                <el-form-item label="默认试用天数">
                  <el-input-number 
                    v-model="form.default_trial_days" 
                    :min="0" 
                    class="!w-full"
                    controls-position="right"
                  />
                  <p class="text-xs text-gray-400 mt-2">新用户注册后获得的初始订阅时长（天）。设为 0 则无试用。</p>
                </el-form-item>

                <el-form-item label="入库通知群组">
                  <el-input
                    v-model="form.notify_group_link"
                    placeholder="https://t.me/your_notify_group"
                    clearable
                  />
                  <p class="text-xs text-gray-400 mt-2">
                    新成员加入群组时展示的通知群组链接。留空则不发送欢迎消息。
                  </p>
                </el-form-item>
              </div>

              <div class="pt-4 border-t border-gray-50 flex justify-end">
                <button 
                  type="button" 
                  @click="handleSaveSettings" 
                  :disabled="saving"
                  class="px-6 py-2 bg-gray-900 text-white rounded-lg hover:bg-black transition-colors font-bold shadow-md hover:shadow-lg disabled:opacity-70 flex items-center gap-2"
                >
                  <span v-if="saving" class="animate-spin w-4 h-4 border-2 border-white/30 border-t-white rounded-full"></span>
                  保存配置
                </button>
              </div>
            </el-form>
          </div>
        </div>
      </div>

      <!-- System Operations -->
      <div class="space-y-6">
        <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden h-full">
          <div class="px-6 py-4 border-b border-gray-50 flex items-center gap-2 bg-gray-50/50">
            <el-icon class="text-gray-400"><Monitor /></el-icon>
            <h3 class="font-bold text-gray-900">系统维护</h3>
          </div>
          
          <div class="p-6 space-y-4">
            <div class="bg-gray-50 rounded-xl p-4 border border-gray-100">
              <div class="flex items-center justify-between mb-2">
                <h4 class="font-bold text-gray-900 text-sm">Emby 连接测试</h4>
                <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse" v-if="testingEmby"></div>
              </div>
              <p class="text-xs text-gray-500 mb-4">检查后端与 Emby 服务器的连接状态。</p>
              <button 
                @click="handleTestEmby"
                :disabled="testingEmby"
                class="w-full py-2 bg-white border border-gray-200 text-gray-700 rounded-lg hover:border-gray-300 hover:bg-gray-50 font-medium text-sm transition-all"
              >
                {{ testingEmby ? '测试中...' : '开始测试' }}
              </button>
            </div>

            <div class="bg-gray-50 rounded-xl p-4 border border-gray-100">
              <div class="flex items-center justify-between mb-2">
                <h4 class="font-bold text-gray-900 text-sm">过期检查任务</h4>
                <div class="w-2 h-2 rounded-full bg-orange-500 animate-pulse" v-if="runningCron"></div>
              </div>
              <p class="text-xs text-gray-500 mb-4">手动触发一次过期账号清理和禁用任务。</p>
              <button 
                @click="handleRunCron"
                :disabled="runningCron"
                class="w-full py-2 bg-white border border-gray-200 text-gray-700 rounded-lg hover:border-gray-300 hover:bg-gray-50 font-medium text-sm transition-all"
              >
                {{ runningCron ? '执行中...' : '立即执行' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.5s ease-out forwards;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}
</style>
