<script setup lang="ts">
const features = [
  {
    icon: '🎬',
    title: '海量影视资源',
    description: '涵盖电影、剧集、纪录片、动漫等多种类型，支持 4K 超高清播放',
  },
  {
    icon: '⚡',
    title: '新番同步追更',
    description: '热门剧集及时更新，不错过每一集精彩内容，第一时间观看最新剧集',
  },
  {
    icon: '📺',
    title: '点播订阅',
    description: '想看什么就订什么，提交订阅申请后自动追番更新，打造您的专属影音库',
  },
  {
    icon: '🎫',
    title: '邀请制社区',
    description: '基于邀请码的精品社区，高质量用户群体，共享优质观影体验',
  },
]
</script>

<template>
  <section id="features" class="py-20 px-4 bg-white dark:bg-gray-900">
    <div class="max-w-7xl mx-auto">
      <!-- Title -->
      <div class="text-center mb-16">
        <h2 class="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
          核心特色
        </h2>
        <p class="text-lg text-gray-600 dark:text-gray-400 max-w-2xl mx-auto">
          优质流媒体资源，极致观影体验
        </p>
      </div>

      <!-- Feature Cards -->
      <div class="grid md:grid-cols-2 lg:grid-cols-4 gap-8">
        <div v-for="(feature, index) in features" :key="index" class="bg-gray-50 dark:bg-gray-800 rounded-2xl p-8 hover:shadow-lg transition-shadow border border-gray-200 dark:border-gray-700">
          <div class="text-5xl mb-4">{{ feature.icon }}</div>
          <h3 class="text-xl font-bold text-gray-900 dark:text-white mb-3">{{ feature.title }}</h3>
          <p class="text-gray-600 dark:text-gray-400 leading-relaxed">{{ feature.description }}</p>
        </div>
      </div>
    </div>
  </section>
</template>
