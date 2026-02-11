<script setup lang="ts">
const currentYear = new Date().getFullYear()
</script>

<template>
  <footer class="bg-gray-50 text-text-primary pt-24 pb-12 px-6 border-t border-gray-200">
    <div class="max-w-4xl mx-auto flex flex-col items-center text-center">
      
      <!-- Brand & Slogan -->
      <div class="flex items-center gap-2 mb-6">
        <div class="w-10 h-10 bg-ember rounded-xl flex items-center justify-center text-white font-black text-xl shadow-lg shadow-ember/20">E</div>
        <span class="text-3xl font-black tracking-tighter text-text-primary">EMBER</span>
      </div>
      
      <p class="text-text-secondary text-lg mb-10 max-w-lg leading-relaxed font-medium">
        连接您的媒体世界。<br/>简单，优雅，自由。
      </p>

      <!-- Community Button -->
      <a href="https://t.me/NextNewEP_emby_chat" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-3 px-6 py-3 bg-white hover:bg-gray-50 text-text-primary rounded-xl transition-all font-bold border border-gray-200 hover:border-gray-300 shadow-sm hover:shadow-md mb-16 group">
        <i class="el-icon-chat-dot-round text-xl text-ember group-hover:scale-110 transition-transform"></i>
        <span>加入 Telegram 社区</span>
      </a>

      <!-- Bottom Info -->
      <div class="w-full pt-8 border-t border-gray-200 flex flex-col md:flex-row justify-between items-center gap-4 text-xs text-text-secondary font-medium tracking-wide">
        <p>© {{ currentYear }} Ember Media. All rights reserved.</p>
        <div class="flex gap-8">
          <a href="#" class="hover:text-ember transition-colors">隐私政策</a>
          <a href="#" class="hover:text-ember transition-colors">服务条款</a>
        </div>
      </div>

    </div>
  </footer>
</template>
