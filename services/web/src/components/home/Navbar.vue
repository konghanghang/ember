<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const mobileMenuOpen = ref(false)
const scrolled = ref(false)

const handleScroll = () => {
  scrolled.value = window.scrollY > 20
}

onMounted(() => {
  window.addEventListener('scroll', handleScroll)
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
})
</script>

<template>
  <nav 
    class="fixed top-0 left-0 right-0 z-50 transition-all duration-300 border-b"
    :class="[
      scrolled ? 'bg-white/90 backdrop-blur-lg border-gray-200 py-3' : 'bg-transparent border-transparent py-6'
    ]"
  >
    <div class="max-w-[1400px] mx-auto px-6">
      <div class="flex justify-between items-center">
        <!-- Logo -->
        <router-link to="/" class="flex items-center gap-2 group">
          <div class="w-8 h-8 bg-ember rounded-lg flex items-center justify-center text-white font-black text-lg transform group-hover:rotate-12 transition-transform">E</div>
          <span class="text-2xl font-black tracking-tighter text-gray-900 group-hover:text-ember transition-colors">EMBER</span>
        </router-link>

        <!-- Desktop Navigation -->
        <div class="hidden md:flex items-center gap-4">
          <a href="https://t.me/NextNewEP_emby_chat" target="_blank" class="text-ember hover:text-red-700 transition-colors p-2" title="加入 Telegram 社区">
            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
            </svg>
          </a>
          <router-link to="/login" class="text-sm font-bold text-gray-900 hover:text-ember transition-colors">
            登录
          </router-link>
          <router-link to="/register" class="px-5 py-2.5 bg-ember text-white text-sm font-bold rounded-lg hover:bg-red-700 transition-colors shadow-lg hover:shadow-ember/20">
            立即体验
          </router-link>
        </div>

        <!-- Mobile Menu Button -->
        <button class="md:hidden p-2 text-gray-900" @click="mobileMenuOpen = !mobileMenuOpen">
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path v-if="mobileMenuOpen" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
        </button>
      </div>

      <!-- Mobile Menu -->
      <div 
        v-show="mobileMenuOpen" 
        class="absolute top-full left-0 right-0 bg-white border-b border-gray-200 p-6 md:hidden shadow-xl"
      >
        <div class="flex flex-col gap-4">
          <router-link to="/login" class="text-lg font-bold text-gray-900" @click="mobileMenuOpen = false">登录</router-link>
          <router-link to="/register" class="text-lg font-bold text-ember" @click="mobileMenuOpen = false">立即体验</router-link>
        </div>
      </div>
    </div>
  </nav>
</template>
