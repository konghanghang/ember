<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/store/auth'
import { approveSubscription, rejectSubscription } from '@/api/admin'
import { deleteSubscription, getSubscriptions } from '@/api/console'
import type { Subscription, SubscriptionStatus } from '@/types/api'

const router = useRouter()
const authStore = useAuthStore()

const subscriptions = ref<Subscription[]>([])
const loading = ref(false)
const total = ref(0)
const queryParams = ref<{
  page: number
  pageSize: number
  status?: SubscriptionStatus
}>({
  page: 1,
  pageSize: 12
})

const statusFilter = ref<'' | SubscriptionStatus>('')

const canReview = computed(() => authStore.isAdmin)

const statusTagType = (status: SubscriptionStatus) => {
  const map: Record<SubscriptionStatus, 'warning' | 'success' | 'info'> = {
    PENDING: 'warning',
    APPROVED: 'success',
    REJECTED: 'info',
    EXPIRED: 'info'
  }
  return map[status] || 'info'
}

const statusText = (status: SubscriptionStatus) => {
  const map: Record<SubscriptionStatus, string> = {
    PENDING: '待审核',
    APPROVED: '已批准',
    REJECTED: '已拒绝',
    EXPIRED: '已过期'
  }
  return map[status] || status
}

const mediaTypeText = (type: string) => {
  return type === 'MOVIE' ? '电影' : '剧集'
}

const fetchData = async () => {
  loading.value = true
  try {
    const params = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize,
      ...(statusFilter.value ? { status: statusFilter.value } : {})
    }
    const res = await getSubscriptions(params)
    subscriptions.value = res.data || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

const handleStatusChange = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleSizeChange = (size: number) => {
  queryParams.value.pageSize = size
  queryParams.value.page = 1
  fetchData()
}

const handlePageChange = (page: number) => {
  queryParams.value.page = page
  fetchData()
}

const handleApprove = async (id: string) => {
  try {
    await approveSubscription(id)
    ElMessage.success('已批准')
    fetchData()
  } catch {
    // handled by interceptor
  }
}

const handleReject = async (id: string) => {
  try {
    await ElMessageBox.confirm('确定拒绝该订阅申请吗？', '提示', { type: 'warning' })
    await rejectSubscription(id)
    ElMessage.success('已拒绝')
    fetchData()
  } catch {
    // cancelled
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

onMounted(fetchData)
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <h2 class="title">订阅管理</h2>
        <div class="actions">
          <el-radio-group v-model="statusFilter" @change="handleStatusChange">
            <el-radio-button label="">全部</el-radio-button>
            <el-radio-button label="PENDING">待审核</el-radio-button>
            <el-radio-button label="APPROVED">已批准</el-radio-button>
            <el-radio-button label="REJECTED">已拒绝</el-radio-button>
          </el-radio-group>
          <el-button type="primary" @click="router.push('/console/subscriptions/new')">
            + 提交新订阅
          </el-button>
        </div>
      </div>
    </template>

    <el-skeleton :loading="loading" animated :count="6">
      <template #template>
        <div class="grid">
          <div v-for="i in 6" :key="i" class="skeleton-card">
            <el-skeleton-item variant="image" style="height: 300px; width: 100%" />
            <div style="padding: 12px">
              <el-skeleton-item variant="h3" style="width: 70%" />
              <el-skeleton-item variant="text" style="width: 50%; margin-top: 8px" />
            </div>
          </div>
        </div>
      </template>

      <template #default>
        <el-empty v-if="subscriptions.length === 0" description="暂无订阅" />

        <div v-else class="grid">
          <div v-for="sub in subscriptions" :key="sub.id" class="sub-card">
            <div class="poster-wrapper">
              <img
                v-if="sub.posterPath"
                :src="`https://image.tmdb.org/t/p/w300${sub.posterPath}`"
                class="poster"
                :alt="sub.name"
              />
              <div v-else class="no-poster">无封面</div>

              <div class="actions-overlay" v-if="sub.status === 'PENDING'">
                <template v-if="canReview">
                  <el-button type="success" size="small" @click.stop="handleApprove(sub.id)">批准</el-button>
                  <el-button type="danger" size="small" @click.stop="handleReject(sub.id)">拒绝</el-button>
                </template>
                <template v-else>
                  <el-button type="danger" size="small" @click.stop="handleDelete(sub.id)">删除</el-button>
                </template>
              </div>
            </div>

            <div class="content">
              <div class="title-row">
                <div class="name" :title="sub.name">{{ sub.name }}</div>
              </div>
              <div class="meta">
                <el-tag size="small">{{ mediaTypeText(sub.type) }}</el-tag>
                <el-tag size="small" :type="statusTagType(sub.status)">{{ statusText(sub.status) }}</el-tag>
              </div>
              <div v-if="canReview && sub.user?.username" class="owner">
                申请人: {{ sub.user.username }}
              </div>
            </div>
          </div>
        </div>
      </template>
    </el-skeleton>

    <div class="pagination-wrap">
      <el-pagination
        :current-page="queryParams.page"
        :page-size="queryParams.pageSize"
        :page-sizes="[12, 24, 48]"
        :total="total"
        layout="total, sizes, prev, pager, next"
        @size-change="handleSizeChange"
        @current-change="handlePageChange"
      />
    </div>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.title {
  margin: 0;
  font-size: 20px;
}
.actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 20px;
}
.sub-card {
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid #e5e7eb;
  background: #fff;
}
.poster-wrapper {
  position: relative;
  height: 300px;
  background: #f3f4f6;
}
.poster {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.no-poster {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
}
.actions-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: rgba(0, 0, 0, 0.45);
  opacity: 0;
  transition: opacity 0.2s ease;
}
.poster-wrapper:hover .actions-overlay {
  opacity: 1;
}
.content {
  padding: 12px;
}
.title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.name {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}
.owner {
  margin-top: 8px;
  color: #6b7280;
  font-size: 12px;
}
.pagination-wrap {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
.skeleton-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
}
</style>
