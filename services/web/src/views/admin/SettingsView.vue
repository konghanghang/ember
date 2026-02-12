<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSystemInfo, testEmbyConnection, runCronJob, getSettings, updateSetting } from '@/api/admin'

const info = ref({
  userCount: 0,
  activeUserCount: 0,
  redemptionCodeCount: 0
})

const form = ref({
  registration_mode: 'open',
  default_trial_days: 7
})

const loading = ref(false)
const saving = ref(false)

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
  if (mode?.value) form.value.registration_mode = mode.value
  if (trial?.value) form.value.default_trial_days = Number(trial.value) || 7
}

const handleSaveSettings = async () => {
  saving.value = true
  try {
    await updateSetting('registration_mode', { value: form.value.registration_mode })
    await updateSetting('default_trial_days', { value: String(form.value.default_trial_days) })
    ElMessage.success('配置保存成功')
  } finally {
    saving.value = false
  }
}

const handleTestEmby = async () => {
  try {
    const res = await testEmbyConnection()
    ElMessage.success(res.message || '连接正常')
  } catch {
    // handled by interceptor
  }
}

const handleRunCron = async () => {
  try {
    const res = await runCronJob()
    ElMessage.success((res as unknown as { message?: string }).message || '任务执行成功')
  } catch {
    // handled by interceptor
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
  <div class="settings-container" v-loading="loading">
    <el-card class="mb-20" header="系统配置">
      <el-form inline>
        <el-form-item label="注册模式">
          <el-select v-model="form.registration_mode" style="width: 200px">
            <el-option label="开放注册" value="open" />
            <el-option label="邀请注册" value="invite" />
          </el-select>
        </el-form-item>

        <el-form-item label="默认试用天数">
          <el-input-number v-model="form.default_trial_days" :min="1" />
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="saving" @click="handleSaveSettings">保存配置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-row :gutter="20">
      <el-col :span="8">
        <el-card shadow="hover">
          <template #header>用户总数</template>
          <div class="stat-value">{{ info.userCount }}</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover">
          <template #header>活跃用户</template>
          <div class="stat-value green">{{ info.activeUserCount }}</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover">
          <template #header>兑换码数</template>
          <div class="stat-value blue">{{ info.redemptionCodeCount }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="mt-20" header="系统操作">
      <el-button type="primary" @click="handleTestEmby">测试 Emby 连接</el-button>
      <el-button type="warning" @click="handleRunCron">手动执行过期检查</el-button>
    </el-card>
  </div>
</template>

<style scoped>
.settings-container {
  padding: 20px 0;
}
.mb-20 {
  margin-bottom: 20px;
}
.mt-20 {
  margin-top: 20px;
}
.stat-value {
  font-size: 24px;
  font-weight: bold;
  text-align: center;
}
.green { color: #67c23a; }
.blue { color: #409eff; }
</style>
