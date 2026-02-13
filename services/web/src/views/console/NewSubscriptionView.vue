<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createSubscription } from '@/api/console'
import TmdbSearch from '@/components/TmdbSearch.vue'
import type { CreateSubscriptionRequest, MediaType, TmdbSelection } from '@/types/api'

const router = useRouter()
const form = ref<CreateSubscriptionRequest>({
  type: 'MOVIE' as MediaType,
  name: '',
  tmdbId: '',
  posterPath: '',
  note: ''
})
const loading = ref(false)

const handleSelectMedia = (media: TmdbSelection) => {
  form.value.name = media.name
  form.value.tmdbId = media.tmdbId
  form.value.posterPath = media.posterPath
}

const handleSubmit = async () => {
  if (!form.value.name || !form.value.tmdbId) {
    ElMessage.warning('请先搜索并选择影视作品')
    return
  }

  loading.value = true
  try {
    await createSubscription(form.value)
    ElMessage.success('提交成功')
    router.push('/console/subscriptions')
  } catch {
    ElMessage.error('提交失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-card header="提交新订阅" style="max-width: 600px; margin: 0 auto;">
    <el-form :model="form" label-width="80px">
      <el-form-item label="类型">
        <el-radio-group v-model="form.type">
          <el-radio-button label="MOVIE">电影</el-radio-button>
          <el-radio-button label="TV">电视剧</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <el-form-item label="搜索">
        <TmdbSearch
          :type="form.type === 'MOVIE' ? 'movie' : 'tv'"
          @select="handleSelectMedia"
        />
      </el-form-item>

      <div v-if="form.name" class="selected-media">
        <p>已选择: <strong>{{ form.name }}</strong> (ID: {{ form.tmdbId }})</p>
      </div>

      <el-form-item label="备注">
        <el-input
          v-model="form.note"
          type="textarea"
          placeholder="可选备注，如：希望能尽快下载"
        />
      </el-form-item>

      <el-form-item>
        <el-button @click="router.back()">取消</el-button>
        <el-button type="primary" :loading="loading" @click="handleSubmit">提交</el-button>
      </el-form-item>
    </el-form>
  </el-card>
</template>

<style scoped>
.selected-media {
  margin: 0 0 20px 80px;
  padding: 10px;
  background: #f0f9eb;
  border: 1px solid #e1f3d8;
  color: #67c23a;
  border-radius: 4px;
}
</style>
