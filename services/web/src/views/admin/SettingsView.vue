<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSystemInfo, testEmbyConnection, runCronJob } from '@/api/admin'

const info = ref({
  userCount: 0,
  activeUserCount: 0,
  inviteCount: 0
})

const fetchInfo = async () => {
  const res = await getSystemInfo()
  if (res.info) {
    info.value = res.info
  }
}

const handleTestEmby = async () => {
  try {
    const res = await testEmbyConnection()
    ElMessage.success(res.message || '连接正常')
  } catch {
    // error handled
  }
}

const handleRunCron = async () => {
  try {
    const res = await runCronJob()
    ElMessage.success(res.message || '任务执行成功')
  } catch {
    // error
  }
}

onMounted(() => {
  fetchInfo()
})
</script>

<template>
  <div class="settings-container">
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
          <template #header>邀请码数</template>
          <div class="stat-value blue">{{ info.inviteCount }}</div>
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
