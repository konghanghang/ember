<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  Check, 
  Close, 
  Delete, 
  Plus, 
  Refresh, 
  VideoPlay,
  Film
} from '@element-plus/icons-vue'
import EmberEmptyStateCard from '@/components/ember/feedback/EmberEmptyStateCard.vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
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
  { label: '已通过', value: 'APPROVED' },
  { label: '已拒绝', value: 'REJECTED' },
  { label: '已入库', value: 'INGESTED' },
]
const statusTabs = computed(() => statusOptions.map((opt) => ({ key: String(opt.value), label: opt.label })))

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

const formatSubscriptionTitle = (sub: Subscription) => {
  if (sub.type === 'TV' && sub.season > 0) {
    return `${sub.name} 第 ${sub.season} 季`
  }
  return sub.name
}

const handleApprove = async (sub: Subscription) => {
  try {
    await approveSubscription(sub.id)
    ElMessage.success(`已批准: ${formatSubscriptionTitle(sub)}`)
    fetchData()
  } catch {
    // handled
  }
}

const handleReject = async (sub: Subscription) => {
  try {
    const { value } = await ElMessageBox.prompt(`请输入拒绝 "${formatSubscriptionTitle(sub)}" 的原因`, '拒绝订阅', {
      confirmButtonText: '提交拒绝',
      cancelButtonText: '取消',
      inputPlaceholder: '请填写明确的拒绝原因',
      inputPattern: /\S+/,
      inputErrorMessage: '拒绝原因不能为空',
      type: 'warning'
    })
    await rejectSubscription(sub.id, value)
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
      isAdminDelete ? `确定删除 "${formatSubscriptionTitle(sub)}" 的订阅记录吗？此操作不可恢复。` : `确定取消 "${formatSubscriptionTitle(sub)}" 的订阅吗？`,
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
    case 'APPROVED': return 'bg-sky-500'
    case 'REJECTED': return 'bg-red-500'
    case 'INGESTED': return 'bg-emerald-500'
    case 'EXPIRED': return 'bg-gray-400'
    default: return 'bg-gray-400'
  }
}

const getStatusText = (status: SubscriptionStatus) => {
  switch (status) {
    case 'PENDING': return '审核中'
    case 'APPROVED': return '已通过，等待入库'
    case 'REJECTED': return '已拒绝'
    case 'INGESTED': return '已入库'
    case 'EXPIRED': return '已过期'
    default: return status
  }
}

const getStatusBadgeText = (status: SubscriptionStatus) => {
  switch (status) {
    case 'APPROVED': return '已通过'
    default: return getStatusText(status)
  }
}

const padNumber = (value: number) => value.toString().padStart(2, '0')

const formatCardDate = (value?: string | null) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getFullYear()}.${padNumber(date.getMonth() + 1)}.${padNumber(date.getDate())}`
}

const formatCompactDateTime = (value?: string | null) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${padNumber(date.getMonth() + 1)}-${padNumber(date.getDate())} ${padNumber(date.getHours())}:${padNumber(date.getMinutes())}`
}

const formatTime = (value?: string | null) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

const getImageUrl = (path?: string) => {
  return path ? `https://image.tmdb.org/t/p/w300${path}` : 'https://via.placeholder.com/300x450?text=No+Poster'
}

onMounted(fetchData)
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard
      title="订阅管理"
      description="查看和管理您的影视订阅请求"
    >
      <template #titleSuffix>
        <span class="text-xs font-normal text-gray-500 bg-gray-100 px-2 py-1 rounded-full">{{ total }} 个订阅</span>
      </template>
      <template #actions>
        <div class="flex items-center gap-3 w-full md:w-auto overflow-x-auto pb-2 md:pb-0">
          <EmberSegmentTabs
            v-model="queryParams.status"
            :tabs="statusTabs"
            :full-width="false"
          />

          <button
            @click="fetchData" 
            class="inline-flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 cursor-pointer"
            aria-label="刷新订阅列表"
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
      </template>
    </EmberPageHeaderCard>

    <!-- Content -->
    <div v-loading="loading" class="min-h-[300px]">
      <div v-if="subscriptions.length > 0" class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-5 md:gap-6 animate-fade-in-up">
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
            <div class="absolute inset-0 bg-gradient-to-t from-black via-black/70 via-black/15 to-transparent opacity-90"></div>

            <!-- Status Badge -->
            <div class="absolute top-2 right-2">
              <span 
                class="px-2 py-0.5 rounded text-[10px] font-bold text-white shadow-sm backdrop-blur-md"
                :class="getStatusColor(sub.status)"
              >
                {{ getStatusBadgeText(sub.status) }}
              </span>
            </div>

            <!-- Type Icon -->
            <div class="absolute left-2 top-2 text-white/80">
              <el-icon v-if="sub.type === 'MOVIE'" :size="16"><Film /></el-icon>
              <el-icon v-else :size="16"><VideoPlay /></el-icon>
            </div>

            <!-- Inline Details -->
            <div class="absolute inset-x-0 bottom-0 p-3 text-white">
              <h3 class="line-clamp-2 text-sm font-bold leading-5 drop-shadow-sm" :title="formatSubscriptionTitle(sub)">
                {{ formatSubscriptionTitle(sub) }}
              </h3>
              <div class="mt-1 flex items-center gap-2 text-[11px] text-white/70">
                <span>{{ formatCardDate(sub.createdAt) }}</span>
                <span v-if="isAdmin && sub.user" class="max-w-[88px] truncate" :title="sub.user.username">
                  {{ sub.user.username }}
                </span>
              </div>

              <div class="mt-2 flex flex-wrap gap-1.5">
                <span
                  v-if="sub.status === 'PENDING'"
                  class="inline-flex items-center rounded-full bg-amber-500/90 px-2 py-1 text-[10px] font-semibold text-white shadow-sm"
                >
                  待审核
                </span>
                <span
                  v-if="sub.reviewedAt"
                  class="inline-flex items-center rounded-full bg-white/15 px-2 py-1 text-[10px] font-medium text-white/90 backdrop-blur-sm"
                  :title="formatTime(sub.reviewedAt)"
                >
                  审核 {{ formatCompactDateTime(sub.reviewedAt) }}
                </span>
                <span
                  v-if="sub.status === 'APPROVED'"
                  class="inline-flex items-center rounded-full bg-sky-500/85 px-2 py-1 text-[10px] font-semibold text-white shadow-sm"
                >
                  待入库
                </span>
                <span
                  v-if="sub.ingestedAt"
                  class="inline-flex items-center rounded-full bg-emerald-500/85 px-2 py-1 text-[10px] font-semibold text-white shadow-sm"
                  :title="formatTime(sub.ingestedAt)"
                >
                  入库 {{ formatCompactDateTime(sub.ingestedAt) }}
                </span>
                <el-tooltip
                  v-if="sub.rejectReason"
                  :content="sub.rejectReason"
                  placement="top"
                  effect="light"
                >
                  <span class="inline-flex cursor-help items-center rounded-full bg-red-500/85 px-2 py-1 text-[10px] font-semibold text-white shadow-sm">
                    原因
                  </span>
                </el-tooltip>
                <el-tooltip
                  v-if="sub.mpError"
                  :content="sub.mpError"
                  placement="top"
                  effect="light"
                >
                  <span class="inline-flex cursor-help items-center rounded-full bg-amber-500/85 px-2 py-1 text-[10px] font-semibold text-white shadow-sm">
                    异常
                  </span>
                </el-tooltip>
                <el-tooltip
                  v-if="sub.note"
                  :content="sub.note"
                  placement="top"
                  effect="light"
                >
                  <span class="inline-flex cursor-help items-center rounded-full bg-white/15 px-2 py-1 text-[10px] font-medium text-white/85 backdrop-blur-sm">
                    备注
                  </span>
                </el-tooltip>
              </div>
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
        </div>
      </div>

      <!-- Empty State -->
      <EmberEmptyStateCard
        v-else
        :icon="Film"
        title="暂无订阅记录"
        description="您还没有提交过任何订阅请求。"
      >
        <template #actions>
          <button 
            @click="router.push('/console/subscriptions/new')"
            class="px-6 py-2 bg-white border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 font-bold transition-colors cursor-pointer"
          >
            去添加
          </button>
        </template>
      </EmberEmptyStateCard>
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
