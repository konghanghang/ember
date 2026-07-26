<template>
  <div class="min-h-screen flex items-center justify-center bg-cinema-bg px-4">
    <div class="w-full max-w-md text-center panel-clean rounded-2xl p-8 md:p-12">
      <h1 class="text-9xl font-bold text-gray-200">404</h1>
      <h2 class="text-3xl font-bold text-text-primary mt-4">页面未找到</h2>
      <p class="text-text-secondary mt-4 mb-8">抱歉，您访问的页面不存在</p>
      <router-link to="/" class="btn-ember px-6 py-3 rounded-xl font-medium inline-flex items-center cursor-pointer">
        返回首页
      </router-link>
    </div>
  </div>
</template>
