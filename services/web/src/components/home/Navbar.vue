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
          <router-link to="/user/login" class="text-sm font-bold text-gray-900 hover:text-ember transition-colors">
            登录
          </router-link>
          <router-link to="/register" class="px-5 py-2.5 bg-gray-900 text-white text-sm font-bold rounded-lg hover:bg-ember transition-colors shadow-lg hover:shadow-ember/20">
            注册体验
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
          <router-link to="/user/login" class="text-lg font-bold text-gray-900" @click="mobileMenuOpen = false">登录</router-link>
          <router-link to="/register" class="text-lg font-bold text-ember" @click="mobileMenuOpen = false">注册体验</router-link>
        </div>
      </div>
    </div>
  </nav>
</template>
