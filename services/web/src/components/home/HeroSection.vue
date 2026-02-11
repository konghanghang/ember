<script setup lang="ts">
</script>

<template>
  <section class="relative pt-32 pb-20 px-4 bg-gradient-to-br from-orange-50 to-amber-100 dark:from-gray-900 dark:to-gray-800">
    <div class="max-w-7xl mx-auto text-center">
      <!-- Main Title -->
      <h1 class="text-5xl md:text-6xl lg:text-7xl font-bold text-gray-900 dark:text-white mb-6">
        Ember
      </h1>

      <!-- Subtitle -->
      <p class="text-2xl md:text-3xl text-gray-700 dark:text-gray-200 mb-4">
        你的私人流媒体影音库
      </p>

      <p class="text-lg text-gray-600 dark:text-gray-400 mb-8 max-w-2xl mx-auto">
        海量 4K 高清资源，热门剧集同步更新
      </p>

      <p class="text-base text-gray-500 dark:text-gray-500 mb-12">
        基于优化线路 · 💰 18元/月 · 国内直连
      </p>

      <!-- CTA Buttons -->
      <div class="flex flex-col sm:flex-row gap-4 justify-center items-center">
        <a href="https://t.me/NextNewEP_emby_chat" target="_blank" rel="noopener noreferrer" class="px-8 py-4 bg-orange-600 text-white rounded-lg hover:bg-orange-700 transition-colors font-medium text-lg shadow-lg hover:shadow-xl">
          📱 加入 Telegram 群组
        </a>
        <router-link to="/user/login" class="px-8 py-4 bg-white dark:bg-gray-800 text-gray-900 dark:text-white rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors font-medium text-lg border border-gray-200 dark:border-gray-700">
          用户登录
        </router-link>
      </div>

      <!-- Hint Text -->
      <p class="mt-8 text-sm text-gray-500 dark:text-gray-400">
        已有账号？
        <router-link to="/user/login" class="text-orange-600 dark:text-orange-400 hover:underline ml-1">
          立即登录
        </router-link>
      </p>
    </div>
  </section>
</template>
