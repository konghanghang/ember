<script setup lang="ts">
const currentYear = new Date().getFullYear()
</script>

<template>
  <footer class="bg-gray-900 text-gray-400 py-12 px-4">
    <div class="max-w-7xl mx-auto">
      <div class="grid md:grid-cols-4 gap-8 mb-8">
        <div class="md:col-span-2">
          <h3 class="text-2xl font-bold text-white mb-3">Ember</h3>
          <p class="text-sm leading-relaxed mb-4">
            专注于提供优质流媒体影音体验的用户管理平台。海量 4K 资源，热门剧集同步更新，邀请制精品社区。
          </p>
          <a href="https://t.me/NextNewEP_emby_chat" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-2 px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 transition-colors text-sm font-medium">
            📱 Telegram 群组
          </a>
        </div>

        <div>
          <h4 class="text-white font-semibold mb-3">快捷链接</h4>
          <ul class="space-y-2 text-sm">
            <li><a href="#features" class="hover:text-white transition-colors">核心特色</a></li>
            <li><a href="#pricing" class="hover:text-white transition-colors">价格方案</a></li>
            <li><a href="#faq" class="hover:text-white transition-colors">常见问题</a></li>
          </ul>
        </div>

        <div>
          <h4 class="text-white font-semibold mb-3">用户入口</h4>
          <ul class="space-y-2 text-sm">
            <li><router-link to="/user/login" class="hover:text-white transition-colors">用户登录</router-link></li>
            <li><router-link to="/register" class="hover:text-white transition-colors">新用户注册</router-link></li>
            <li><router-link to="/login" class="hover:text-white transition-colors">管理员登录</router-link></li>
          </ul>
        </div>
      </div>

      <div class="border-t border-gray-800 pt-8 text-center text-sm">
        <p>© {{ currentYear }} Ember. All rights reserved.</p>
      </div>
    </div>
  </footer>
</template>
