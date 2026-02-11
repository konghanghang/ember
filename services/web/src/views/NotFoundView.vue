<template>
  <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900 dark:to-gray-800 px-4">
    <div class="text-center">
      <h1 class="text-9xl font-bold text-gray-300 dark:text-gray-700">404</h1>
      <h2 class="text-3xl font-bold text-gray-900 dark:text-white mt-4">页面未找到</h2>
      <p class="text-gray-600 dark:text-gray-400 mt-4 mb-8">抱歉，您访问的页面不存在</p>
      <router-link to="/" class="inline-flex items-center px-6 py-3 bg-orange-600 hover:bg-orange-700 text-white font-medium rounded-lg transition-colors">
        返回首页
      </router-link>
    </div>
  </div>
</template>
