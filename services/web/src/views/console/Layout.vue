<script setup lang="ts">
import { onMounted, provide, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import HelpWidget from '@/components/console/HelpWidget.vue'
import Sidebar from '@/components/console/Sidebar.vue'
import TopBar from '@/components/console/TopBar.vue'
import { refreshConsoleProfileKey, type RefreshConsoleProfile } from '@/constants/consoleProfile'
import { useConsoleStore } from '@/store/console'
import { useUserStore } from '@/store/user'

const sidebarOpen = ref(false)
const route = useRoute()
const consoleStore = useConsoleStore()
const userStore = useUserStore()

const toggleSidebar = () => {
  sidebarOpen.value = !sidebarOpen.value
}

const refreshConsoleProfile: RefreshConsoleProfile = async () => {
  try {
    await userStore.fetchProfile()
    await consoleStore.fetchAccountLinks()
  } catch {
    // handled by request interceptor
  }
}

provide(refreshConsoleProfileKey, refreshConsoleProfile)

onMounted(refreshConsoleProfile)

watch(
  () => route.fullPath,
  () => {
    sidebarOpen.value = false
  }
)
</script>

<template>
  <div class="flex h-screen overflow-hidden bg-gray-50">
    <!-- Desktop Sidebar -->
    <div class="hidden lg:flex lg:flex-col lg:w-64 lg:fixed lg:inset-y-0 lg:border-r lg:border-gray-200 lg:bg-white lg:pt-0 lg:pb-4">
      <Sidebar />
    </div>

    <!-- Mobile Sidebar (Overlay) -->
    <div v-if="sidebarOpen" class="fixed inset-0 z-40 flex lg:hidden" role="dialog" aria-modal="true">
      <div class="fixed inset-0 bg-gray-600 bg-opacity-75 transition-opacity" @click="sidebarOpen = false"></div>
      <div class="relative flex-1 flex flex-col max-w-xs w-full bg-white transition ease-in-out duration-300 transform translate-x-0">
        <div class="absolute top-0 right-0 -mr-12 pt-2">
          <button
            aria-label="关闭导航菜单"
            @click="sidebarOpen = false"
            class="ml-1 flex h-10 w-10 items-center justify-center rounded-full cursor-pointer focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white"
          >
            <span class="sr-only">关闭导航菜单</span>
            <!-- Icon for close -->
            <svg class="h-6 w-6 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <Sidebar />
      </div>
      <div class="flex-shrink-0 w-14"><!-- Dummy element to force sidebar width --></div>
    </div>

    <!-- Main Content -->
    <div class="flex flex-col flex-1 w-0 overflow-hidden lg:pl-64 transition-all duration-300">
      <TopBar :collapsed="!sidebarOpen" @toggle-sidebar="toggleSidebar" />
      
      <main class="flex-1 relative overflow-y-auto focus:outline-none bg-gray-50/50">
        <div class="py-6 px-4 sm:px-6 lg:px-8 max-w-7xl mx-auto">
          <router-view v-slot="{ Component }">
            <transition name="fade-slide" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </main>

      <HelpWidget />
    </div>
  </div>
</template>

<style scoped>
.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.fade-slide-enter-from,
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(10px);
}
</style>
