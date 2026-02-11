<script setup lang="ts">
import { ref } from 'vue'

const openIndex = ref<number | null>(0)

const faqs = [
  {
    question: '需要什么条件才能流畅观看？',
    answer: '我们使用优化线路，国内用户无需额外工具即可流畅观看。建议使用 10Mbps 以上带宽以获得最佳 4K 观影体验。',
  },
  {
    question: '支持哪些设备？',
    answer: '支持 Emby 全平台客户端，包括 iOS、Android、Apple TV、Android TV、Web 浏览器、Windows、macOS 等。可同时在 5 个设备上观看。',
  },
  {
    question: '如何购买和注册？',
    answer: '加入我们的 Telegram 群组，群内联系管理员购买邀请码（18元/月）。购买后使用邀请码注册账号，即可立即使用。',
  },
  {
    question: '资源更新频率如何？',
    answer: '热门剧集同步更新，Netflix、Disney+ 等平台的新剧通常在播出后数小时内即可观看。电影和纪录片也会持续更新。',
  },
]

const toggle = (index: number) => {
  openIndex.value = openIndex.value === index ? null : index
}
</script>

<template>
  <section id="faq" class="py-20 px-4 bg-white dark:bg-gray-900">
    <div class="max-w-3xl mx-auto">
      <div class="text-center mb-16">
        <h2 class="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
          常见问题
        </h2>
        <p class="text-lg text-gray-600 dark:text-gray-400">
          快速了解我们的服务
        </p>
      </div>

      <div class="space-y-4">
        <div v-for="(faq, index) in faqs" :key="index" class="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
          <button @click="toggle(index)" class="w-full px-6 py-4 text-left bg-gray-50 dark:bg-gray-800 hover:bg-gray-100 dark:hover:bg-gray-750 transition-colors flex justify-between items-center">
            <span class="font-semibold text-gray-900 dark:text-white">{{ faq.question }}</span>
            <svg class="w-5 h-5 text-gray-500 transition-transform" :class="{ 'transform rotate-180': openIndex === index }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          <div v-if="openIndex === index" class="px-6 py-4 bg-white dark:bg-gray-900">
            <p class="text-gray-600 dark:text-gray-400 leading-relaxed">{{ faq.answer }}</p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
