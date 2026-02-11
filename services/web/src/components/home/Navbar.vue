<script setup lang="ts">
import { ref } from 'vue'

const mobileMenuOpen = ref(false)
</script>

<template>
  <nav class="fixed top-0 left-0 right-0 bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-b border-gray-200 dark:border-gray-800 z-50">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex justify-between items-center h-16">
        <!-- Logo -->
        <router-link to="/" class="flex items-center">
          <span class="text-2xl font-bold text-orange-600 dark:text-orange-400">
            Ember
          </span>
        </router-link>

        <!-- Desktop Navigation -->
        <div class="hidden md:flex items-center space-x-8">
          <a href="#features" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">功能介绍</a>
          <a href="#pricing" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">价格方案</a>
          <a href="#faq" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">常见问题</a>
        </div>

        <!-- Right Side Buttons -->
        <div class="hidden md:flex items-center space-x-4">
          <router-link to="/register" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">注册账号</router-link>
          <router-link to="/user/login" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">用户登录</router-link>
          <router-link to="/login" class="px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 transition-colors font-medium">管理员入口</router-link>
        </div>

        <!-- Mobile Menu Button -->
        <button class="md:hidden p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800" @click="mobileMenuOpen = !mobileMenuOpen">
          <svg class="w-6 h-6 text-gray-600 dark:text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path v-if="mobileMenuOpen" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
        </button>
      </div>

      <!-- Mobile Menu -->
      <div v-if="mobileMenuOpen" class="md:hidden py-4 border-t border-gray-200 dark:border-gray-800">
        <div class="flex flex-col space-y-4">
          <a href="#features" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors" @click="mobileMenuOpen = false">功能介绍</a>
          <a href="#pricing" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors" @click="mobileMenuOpen = false">价格方案</a>
          <router-link to="/register" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">注册账号</router-link>
          <router-link to="/user/login" class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors">用户登录</router-link>
          <router-link to="/login" class="px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 transition-colors font-medium text-center">管理员入口</router-link>
        </div>
      </div>
    </div>
  </nav>
</template>
