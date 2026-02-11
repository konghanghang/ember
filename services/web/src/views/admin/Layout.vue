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
      <div class="logo">Ember 管理后台</div>
      <div class="user-info">
        <span>管理员</span>
        <el-button link type="primary" @click="handleLogout">登出</el-button>
      </div>
    </el-header>
    <el-container>
      <el-aside width="200px" class="aside">
        <el-menu router :default-active="$route.path">
          <el-menu-item index="/admin/users">
            <el-icon><User /></el-icon>
            <span>用户管理</span>
          </el-menu-item>
          <el-menu-item index="/admin/invites">
            <el-icon><Ticket /></el-icon>
            <span>邀请码管理</span>
          </el-menu-item>
          <el-menu-item index="/admin/subscriptions">
            <el-icon><VideoPlay /></el-icon>
            <span>订阅管理</span>
          </el-menu-item>
          <el-menu-item index="/admin/settings">
            <el-icon><Setting /></el-icon>
            <span>系统设置</span>
          </el-menu-item>
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
.aside {
  border-right: 1px solid #dcdfe6;
}
.main {
  background-color: #f0f2f5;
  padding: 20px;
}
</style>
