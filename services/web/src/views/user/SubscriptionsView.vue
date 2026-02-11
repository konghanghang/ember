<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getUserSubscriptions, deleteSubscription } from '@/api/user'

const router = useRouter()
const subscriptions = ref([])
const loading = ref(false)

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getUserSubscriptions()
    subscriptions.value = res || []
  } finally {
    loading.value = false
  }
}

const handleDelete = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定取消该订阅吗？', '提示', { type: 'warning' })
    await deleteSubscription(id)
    ElMessage.success('已删除')
    fetchData()
  } catch {
    // cancelled
  }
}

const getStatusTag = (status: string) => {
  const map: Record<string, string> = {
    PENDING: 'warning',
    APPROVED: 'success',
    REJECTED: 'info'
  }
  return map[status] || 'info'
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>我的订阅</span>
        <el-button type="primary" icon="Plus" @click="router.push('/user/subscriptions/new')">
          提交新订阅
        </el-button>
      </div>
    </template>

    <div v-if="subscriptions.length === 0" class="empty">
      <el-empty description="暂无订阅" />
    </div>

    <div v-else class="grid">
      <el-card 
        v-for="sub in subscriptions" 
        :key="sub.id" 
        :body-style="{ padding: '0px' }"
        class="sub-card"
      >
        <div class="poster-wrapper">
          <img 
            v-if="sub.posterPath" 
            :src="`https://image.tmdb.org/t/p/w300${sub.posterPath}`" 
            class="poster"
          />
          <div v-else class="no-poster">无封面</div>
        </div>
        <div class="content">
          <div class="title" :title="sub.name">{{ sub.name }}</div>
          <div class="meta">
            <el-tag size="small">{{ sub.type === 'MOVIE' ? '电影' : '剧集' }}</el-tag>
            <el-tag size="small" :type="getStatusTag(sub.status)">{{ sub.status }}</el-tag>
          </div>
          <div class="actions" v-if="sub.status === 'PENDING'">
            <el-button link type="danger" size="small" @click="handleDelete(sub.id)">删除</el-button>
          </div>
        </div>
      </el-card>
    </div>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 20px;
}
.sub-card {
  position: relative;
}
.poster-wrapper {
  height: 300px;
  background: #eee;
  display: flex;
  justify-content: center;
  align-items: center;
  overflow: hidden;
}
.poster {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.content {
  padding: 14px;
}
.title {
  font-weight: bold;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 8px;
}
.meta {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.actions {
  text-align: right;
}
</style>
