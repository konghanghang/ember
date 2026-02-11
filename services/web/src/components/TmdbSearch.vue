<script setup lang="ts">
import { ref } from 'vue'
import { ElInput } from 'element-plus'
import { searchTmdb } from '@/api/user'

const props = defineProps<{
  type: 'movie' | 'tv'
}>()

const emit = defineEmits(['select'])

const query = ref('')
const loading = ref(false)
const results = ref<any[]>([])
const showResults = ref(false)

const handleSearch = async () => {
  if (!query.value) return
  
  loading.value = true
  try {
    const res = await searchTmdb(query.value, props.type)
    results.value = res.results || []
    showResults.value = true
  } finally {
    loading.value = false
  }
}

const handleSelect = (item: any) => {
  emit('select', {
    tmdbId: item.id.toString(),
    name: item.title, // standardized from backend
    posterPath: item.posterPath
  })
  query.value = item.title
  showResults.value = false
}
</script>

<template>
  <div class="search-container">
    <el-input
      v-model="query"
      placeholder="输入名称搜索..."
      @keyup.enter="handleSearch"
    >
      <template #append>
        <el-button :loading="loading" icon="Search" @click="handleSearch" />
      </template>
    </el-input>

    <div v-if="showResults && results.length" class="results-list">
      <div 
        v-for="item in results" 
        :key="item.id" 
        class="result-item"
        @click="handleSelect(item)"
      >
        <img 
          v-if="item.posterPath" 
          :src="`https://image.tmdb.org/t/p/w92${item.posterPath}`" 
          class="poster-thumb"
        />
        <div class="info">
          <div class="title">{{ item.title }}</div>
          <div class="date">{{ item.releaseDate }}</div>
        </div>
      </div>
    </div>
    
    <div v-if="showResults && results.length === 0" class="no-results">
      未找到结果
    </div>
  </div>
</template>

<style scoped>
.search-container {
  position: relative;
}
.results-list {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  background: white;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  max-height: 300px;
  overflow-y: auto;
  z-index: 10;
  box-shadow: 0 2px 12px 0 rgba(0,0,0,0.1);
}
.result-item {
  display: flex;
  padding: 10px;
  cursor: pointer;
  border-bottom: 1px solid #eee;
}
.result-item:hover {
  background-color: #f5f7fa;
}
.poster-thumb {
  width: 40px;
  height: 60px;
  object-fit: cover;
  margin-right: 10px;
  background: #eee;
}
.title {
  font-weight: bold;
  font-size: 14px;
}
.date {
  font-size: 12px;
  color: #909399;
}
.no-results {
  position: absolute;
  top: 100%;
  width: 100%;
  padding: 10px;
  background: white;
  border: 1px solid #dcdfe6;
  text-align: center;
  color: #909399;
  z-index: 10;
}
</style>
