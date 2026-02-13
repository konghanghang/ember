<script setup lang="ts">
import { User, VideoPlay, Monitor } from '@element-plus/icons-vue'

const features = [
  {
    icon: User,
    title: '自助注册系统',
    description: '告别手动发号。用户可直接注册体验 Emby 服务，权限自动下发，省时省力。',
  },
  {
    icon: VideoPlay,
    title: '订阅与求片',
    description: '内置 TMDB 搜索。用户想看什么直接搜，管理员一键批准，不再需要在群里手动统计需求。',
  },
  {
    icon: Monitor,
    title: '自动化同步',
    description: '与 MoviePilot 等工具无缝集成。请求批准后自动触发下载任务，资源就绪后自动通知。',
  },
]
</script>

<template>
  <section id="features" class="py-24 px-6 bg-white relative overflow-hidden">
    <!-- Subtle Gradient Bg -->
    <div class="absolute top-0 right-0 w-1/2 h-full bg-gradient-to-l from-gray-50 to-transparent pointer-events-none"></div>

    <div class="max-w-[1100px] mx-auto relative z-10">
      <div class="text-center mb-12">
        <h2 class="text-sm font-bold text-ember tracking-widest uppercase mb-2">功能特性</h2>
        <h3 class="text-3xl font-black text-gray-900">为您打造的完整工作流</h3>
      </div>

      <div class="grid md:grid-cols-3 gap-6">
        <div 
          v-for="(feature, index) in features" 
          :key="index" 
          class="group relative p-8 rounded-2xl bg-white border border-gray-100 shadow-sm hover:shadow-xl hover:border-red-100 hover:-translate-y-1 transition-all duration-300"
        >
          <!-- Icon Container -->
          <div class="w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mb-6 border border-gray-100 group-hover:scale-110 group-hover:bg-ember/10 group-hover:border-ember/20 transition-all duration-300">
            <el-icon class="text-2xl text-gray-600 group-hover:text-ember transition-colors">
              <component :is="feature.icon" />
            </el-icon>
          </div>
          
          <h4 class="text-xl font-bold text-gray-900 mb-3 group-hover:text-ember transition-colors">{{ feature.title }}</h4>
          <p class="text-gray-500 leading-relaxed text-sm">
            {{ feature.description }}
          </p>
        </div>
      </div>
    </div>
  </section>
</template>
