<template>
  <div class="bg-gray-50 border-y border-gray-100 py-4 overflow-hidden">
    <div class="flex whitespace-nowrap animate-marquee text-xs font-bold tracking-widest text-gray-400">
      <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 自动化用户管理</span>
      <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> TMDB 深度集成</span>
      <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 即时请求同步</span>
      <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 安全访问控制</span>
      <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 全平台兼容</span>
      <!-- Duplicate for seamless loop; hidden from screen readers to avoid repeated announcements -->
      <span class="mx-8 flex items-center gap-2" aria-hidden="true"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 自动化用户管理</span>
      <span class="mx-8 flex items-center gap-2" aria-hidden="true"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> TMDB 深度集成</span>
      <span class="mx-8 flex items-center gap-2" aria-hidden="true"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 即时请求同步</span>
      <span class="mx-8 flex items-center gap-2" aria-hidden="true"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 安全访问控制</span>
      <span class="mx-8 flex items-center gap-2" aria-hidden="true"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 全平台兼容</span>
    </div>
  </div>
</template>

<style scoped>
@keyframes marquee {
  0% { transform: translateX(0); }
  100% { transform: translateX(-50%); }
}
.animate-marquee {
  animation: marquee 30s linear infinite;
}
</style>
