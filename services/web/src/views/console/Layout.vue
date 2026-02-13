<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'

const router = useRouter()
const authStore = useAuthStore()

const handleLogout = async () => {
  await authStore.logout()
  ElMessage.success('已登出')
  router.push('/login')
}
</script>

<template>
  <el-container class="layout-container">
    <el-header class="header">
      <div class="logo">Ember 控制台</div>
      <div class="user-info">
        <el-button link @click="router.push('/')">首页</el-button>
        <el-tag v-if="authStore.isAdmin" type="danger" size="small">管理员</el-tag>
        <el-button link type="primary" @click="handleLogout">登出</el-button>
      </div>
    </el-header>
    <el-container>
      <el-aside width="220px" class="aside">
        <el-menu router :default-active="$route.path">
          <el-menu-item index="/console/dashboard">
            <el-icon><Odometer /></el-icon>
            <span>我的账号</span>
          </el-menu-item>
          <el-menu-item index="/console/subscriptions">
            <el-icon><VideoPlay /></el-icon>
            <span>订阅管理</span>
          </el-menu-item>

          <el-menu-item-group v-if="authStore.isAdmin" title="管理">
            <el-menu-item index="/console/users">
              <el-icon><User /></el-icon>
              <span>用户管理</span>
            </el-menu-item>
            <el-menu-item index="/console/redemption-codes">
              <el-icon><Ticket /></el-icon>
              <span>兑换码管理</span>
            </el-menu-item>
            <el-menu-item index="/console/settings">
              <el-icon><Setting /></el-icon>
              <span>系统设置</span>
            </el-menu-item>
          </el-menu-item-group>
        </el-menu>
      </el-aside>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.layout-container {
  height: 100vh;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #dcdfe6;
}
.logo {
  font-size: 20px;
  font-weight: bold;
}
.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
}
.aside {
  border-right: 1px solid #dcdfe6;
}
.main {
  background-color: #f0f2f5;
  padding: 20px;
}
</style>
