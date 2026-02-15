<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  Check, 
  Close, 
  Delete, 
  Plus, 
  Search, 
  Refresh, 
  Filter,
  VideoPlay,
  Film,
  UserFilled
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import { approveSubscription, rejectSubscription, deleteSubscriptionAsAdmin } from '@/api/admin'
import { deleteSubscription, getSubscriptions } from '@/api/console'
import type { Subscription, SubscriptionStatus } from '@/types/api'

const router = useRouter()
const authStore = useAuthStore()

const subscriptions = ref<Subscription[]>([])
const loading = ref(false)
const total = ref(0)
const queryParams = ref({
  page: 1,
  pageSize: 20,
  status: '' as SubscriptionStatus | ''
})

const isAdmin = computed(() => authStore.isAdmin)

const statusOptions = [
  { label: '全部', value: '' },
  { label: '待审核', value: 'PENDING' },
  { label: '已批准', value: 'APPROVED' },
  { label: '已拒绝', value: 'REJECTED' },
]

const fetchData = async () => {
  loading.value = true
  try {
    const params: any = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize
    }
    if (queryParams.value.status) {
      params.status = queryParams.value.status
    }
    const res = await getSubscriptions(params)
    subscriptions.value = res.data || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

watch(() => queryParams.value.status, () => {
  queryParams.value.page = 1
  fetchData()
})

const handleApprove = async (sub: Subscription) => {
  try {
    await approveSubscription(sub.id)
    ElMessage.success(`已批准: ${sub.name}`)
    fetchData()
  } catch {
    // handled
  }
}

const handleReject = async (sub: Subscription) => {
  try {
    await ElMessageBox.confirm(`确定拒绝 "${sub.name}" 的订阅申请吗？`, '拒绝确认', { 
      confirmButtonText: '拒绝',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    })
    await rejectSubscription(sub.id)
    ElMessage.success('已拒绝')
    fetchData()
  } catch {
    // cancelled
  }
}

const handleDelete = async (sub: Subscription) => {
  const isAdminDelete = isAdmin.value

  try {
    await ElMessageBox.confirm(
      isAdminDelete ? `确定删除 "${sub.name}" 的订阅记录吗？此操作不可恢复。` : `确定取消 "${sub.name}" 的订阅吗？`,
      isAdminDelete ? '删除确认' : '取消确认',
      {
      confirmButtonText: isAdminDelete ? '确认删除' : '确定取消',
      cancelButtonText: isAdminDelete ? '取消' : '保留',
      type: 'warning'
    })

    if (isAdminDelete) {
      await deleteSubscriptionAsAdmin(sub.id)
      ElMessage.success('已删除订阅记录')
    } else {
      await deleteSubscription(sub.id)
      ElMessage.success('已取消订阅')
    }

    fetchData()
  } catch {
    // cancelled
  }
}

const getStatusColor = (status: SubscriptionStatus) => {
  switch (status) {
    case 'PENDING': return 'bg-yellow-500'
    case 'APPROVED': return 'bg-green-500'
    case 'REJECTED': return 'bg-red-500'
    case 'EXPIRED': return 'bg-gray-400'
    default: return 'bg-gray-400'
  }
}

const getStatusText = (status: SubscriptionStatus) => {
  switch (status) {
    case 'PENDING': return '审核中'
    case 'APPROVED': return '已批准'
    case 'REJECTED': return '已拒绝'
    case 'EXPIRED': return '已过期'
    default: return status
  }
}

const getImageUrl = (path?: string) => {
  return path ? `https://image.tmdb.org/t/p/w300${path}` : 'https://via.placeholder.com/300x450?text=No+Poster'
}

onMounted(fetchData)
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          订阅管理
          <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">{{ total }} 个订阅</span>
        </h1>
        <p class="text-gray-500 text-sm mt-1">查看和管理您的影视订阅请求</p>
      </div>
      
      <div class="flex items-center gap-3 w-full md:w-auto overflow-x-auto pb-2 md:pb-0">
        <div class="flex bg-gray-100 p-1 rounded-xl flex-shrink-0">
          <button 
            v-for="opt in statusOptions"
            :key="opt.value"
            @click="queryParams.status = opt.value as any"
            class="px-4 py-1.5 rounded-lg text-sm font-bold transition-all whitespace-nowrap cursor-pointer"
            :class="queryParams.status === opt.value ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'"
          >
            {{ opt.label }}
          </button>
        </div>
        
          <button
            @click="fetchData" 
            class="p-2 text-gray-500 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors flex-shrink-0 cursor-pointer"
            title="刷新列表"
          >
            <el-icon :size="20"><Refresh /></el-icon>
          </button>

          <button 
            @click="router.push('/console/subscriptions/new')"
            class="btn-ember flex items-center gap-2 px-4 py-2 rounded-lg font-bold shadow-md hover:shadow-lg active:scale-95 flex-shrink-0 whitespace-nowrap cursor-pointer"
          >
            <el-icon><Plus /></el-icon>
            <span>新建订阅</span>
          </button>
        </div>
      </div>

    <!-- Content -->
    <div v-loading="loading" class="min-h-[300px]">
      <div v-if="subscriptions.length > 0" class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-6 animate-fade-in-up">
        <div 
          v-for="sub in subscriptions" 
          :key="sub.id"
          class="group relative bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden hover:shadow-xl hover:border-ember/30 transition-all duration-300"
        >
          <!-- Poster -->
          <div class="aspect-[2/3] relative bg-gray-100 overflow-hidden">
            <img 
              :src="getImageUrl(sub.posterPath)" 
              class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-110"
              loading="lazy"
            />
            
            <!-- Gradient Overlay -->
            <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-60"></div>

            <!-- Status Badge -->
            <div class="absolute top-2 right-2">
              <span 
                class="px-2 py-0.5 rounded text-[10px] font-bold text-white shadow-sm backdrop-blur-md"
                :class="getStatusColor(sub.status)"
              >
                {{ getStatusText(sub.status) }}
              </span>
            </div>

            <!-- Type Icon -->
            <div class="absolute bottom-2 left-2 text-white/80">
              <el-icon v-if="sub.type === 'MOVIE'" :size="16"><Film /></el-icon>
              <el-icon v-else :size="16"><VideoPlay /></el-icon>
            </div>

            <!-- Hover Actions -->
            <div 
              v-if="isAdmin || sub.status === 'PENDING'"
              class="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex flex-col items-center justify-center gap-3 backdrop-blur-sm p-4"
            >
              <template v-if="isAdmin">
                <template v-if="sub.status === 'PENDING'">
                  <button 
                    @click="handleApprove(sub)"
                    class="w-full py-2 bg-green-500 hover:bg-green-600 text-white rounded-lg font-bold text-xs shadow-lg transform translate-y-4 group-hover:translate-y-0 transition-all duration-300 flex items-center justify-center gap-1"
                  >
                    <el-icon><Check /></el-icon> 批准
                  </button>
                  <button 
                    @click="handleReject(sub)"
                    class="w-full py-2 bg-white/20 hover:bg-red-500 text-white rounded-lg font-bold text-xs backdrop-blur-md transform translate-y-4 group-hover:translate-y-0 transition-all duration-300 delay-75 flex items-center justify-center gap-1"
                  >
                    <el-icon><Close /></el-icon> 拒绝
                  </button>
                </template>
                <button 
                  @click="handleDelete(sub)"
                  class="w-full py-2 bg-red-500 hover:bg-red-600 text-white rounded-lg font-bold text-xs shadow-lg transform translate-y-4 group-hover:translate-y-0 transition-all duration-300 delay-100 flex items-center justify-center gap-1"
                >
                  <el-icon><Delete /></el-icon> 删除记录
                </button>
              </template>
              <template v-else>
                <button 
                  @click="handleDelete(sub)"
                  class="w-full py-2 bg-red-500 hover:bg-red-600 text-white rounded-lg font-bold text-xs shadow-lg transform translate-y-4 group-hover:translate-y-0 transition-all duration-300 flex items-center justify-center gap-1"
                >
                  <el-icon><Delete /></el-icon> 取消订阅
                </button>
              </template>
            </div>
          </div>

          <!-- Info -->
          <div class="p-3">
            <h3 class="font-bold text-gray-900 text-sm line-clamp-1 mb-1" :title="sub.name">{{ sub.name }}</h3>
            <div class="flex items-center justify-between text-xs text-gray-500">
              <span>{{ new Date(sub.createdAt).toLocaleDateString() }}</span>
              <div v-if="isAdmin && sub.user" class="flex items-center gap-1" :title="sub.user.username">
                <el-icon><UserFilled /></el-icon>
                <span class="max-w-[60px] truncate">{{ sub.user.username }}</span>
              </div>
            </div>
            
            <div v-if="sub.note" class="mt-2 pt-2 border-t border-gray-100">
              <p class="text-[10px] text-gray-400 line-clamp-2 italic">"{{ sub.note }}"</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else class="flex flex-col items-center justify-center py-20 text-gray-400 bg-white rounded-2xl border border-dashed border-gray-200">
        <div class="w-20 h-20 bg-gray-50 rounded-full flex items-center justify-center mb-4 text-gray-300">
          <el-icon :size="40"><Film /></el-icon>
        </div>
        <p class="text-lg font-medium text-gray-500">暂无订阅记录</p>
        <p class="text-sm mt-1 mb-6">您还没有提交过任何订阅请求</p>
        <button 
          @click="router.push('/console/subscriptions/new')"
          class="px-6 py-2 bg-white border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 font-bold transition-colors cursor-pointer"
        >
          去添加
        </button>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="total > 0" class="flex justify-end pt-4">
      <el-pagination
        v-model:current-page="queryParams.page"
        v-model:page-size="queryParams.pageSize"
        :total="total"
        :page-sizes="[20, 40, 80]"
        layout="total, sizes, prev, pager, next"
        @size-change="fetchData"
        @current-change="fetchData"
        background
      />
    </div>
  </div>
</template>

<style scoped>
.animate-fade-in-up {
  animation: fadeInUp 0.5s ease-out forwards;
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
