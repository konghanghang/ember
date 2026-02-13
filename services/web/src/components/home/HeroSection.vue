<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/store/auth'
import { VideoPlay, Connection, ArrowRight, User } from '@element-plus/icons-vue'

const router = useRouter()
const authStore = useAuthStore()

const handleLoginClick = () => {
  authStore.restoreAuth()
  if (authStore.isAuthenticated) {
    router.push('/console/dashboard')
    return
  }
  router.push('/login')
}
</script>

<template>
  <header class="relative pt-32 pb-20 lg:pt-48 lg:pb-32 px-6 overflow-hidden bg-gray-50">
    <!-- Background Decor -->
    <div class="absolute inset-0 pointer-events-none">
      <div class="absolute top-0 left-1/2 -translate-x-1/2 w-[1000px] h-[600px] bg-red-500/5 rounded-full blur-3xl opacity-50"></div>
      <div class="absolute bottom-0 right-0 w-[800px] h-[600px] bg-orange-500/5 rounded-full blur-3xl opacity-30"></div>
    </div>

    <div class="max-w-7xl mx-auto grid lg:grid-cols-12 gap-12 items-center relative z-10">
      
      <!-- Text Content (Restored from user snippet) -->
      <div class="lg:col-span-6 relative z-10 text-center lg:text-left">
        <div class="inline-flex items-center gap-2 px-3 py-1 bg-red-50 border border-red-100 rounded-full text-xs font-bold tracking-widest uppercase mb-6 text-ember animate-fade-in-up">
          <span class="w-2 h-2 rounded-full bg-ember animate-pulse"></span>
          Ember v1.0 正式上线
        </div>
        
        <h1 class="text-5xl md:text-7xl font-black tracking-tight leading-[1.1] mb-6 animate-fade-in-up" style="animation-delay: 0.1s">
          媒体管理，<br class="hidden lg:block"/>
          <span class="text-ember relative inline-block">
            从未如此优雅。
            <!-- 装饰下划线 -->
            <svg class="absolute -bottom-2 left-0 w-full h-3 text-ember/20" viewBox="0 0 100 10" preserveAspectRatio="none">
              <path d="M0 5 Q 50 10 100 5" stroke="currentColor" stroke-width="8" fill="none" />
            </svg>
          </span>
        </h1>
        
        <p class="text-lg text-gray-500 max-w-lg mx-auto lg:mx-0 mb-8 font-medium leading-relaxed animate-fade-in-up" style="animation-delay: 0.2s">
          为您的 Emby 服务器打造的自动化管家。
          <br class="hidden md:block"/>
          从注册到求片订阅，一切自动完成。
        </p>
        
        <div class="flex flex-col sm:flex-row items-center gap-6 justify-center lg:justify-start animate-fade-in-up" style="animation-delay: 0.3s">
          <button @click="router.push('/register')" class="group relative px-8 py-3.5 bg-ember text-white text-base font-bold tracking-wide rounded-lg transition-all hover:shadow-lg hover:shadow-ember/30 hover:-translate-y-0.5 overflow-hidden">
            <span class="relative z-10 flex items-center justify-center gap-2">
              立即体验
              <el-icon class="transition-transform group-hover:translate-x-1"><ArrowRight /></el-icon>
            </span>
            <div class="absolute inset-0 bg-black/10 translate-y-full group-hover:translate-y-0 transition-transform duration-300"></div>
          </button>
          
          <div class="text-sm text-gray-500 font-medium">
            已有账号？ <router-link to="/login" class="text-ember hover:underline font-bold transition-colors">立即登录</router-link>
          </div>
        </div>
        
        <!-- 信任背书/小数据 -->
        <div class="mt-8 flex items-center justify-center lg:justify-start gap-6 text-sm text-gray-400 font-medium animate-fade-in-up" style="animation-delay: 0.4s">
          <div class="flex items-center gap-2">
            <el-icon><User /></el-icon>
            <span>自助注册</span>
          </div>
          <div class="w-1 h-1 bg-gray-300 rounded-full"></div>
          <div class="flex items-center gap-2">
            <el-icon><VideoPlay /></el-icon>
            <span>一键求片</span>
          </div>
          <div class="w-1 h-1 bg-gray-300 rounded-full"></div>
          <div class="flex items-center gap-2">
            <el-icon><Connection /></el-icon>
            <span>自动同步</span>
          </div>
        </div>
      </div>

      <!-- Right Visual (Visual enhancement, kept for design quality) -->
      <div class="lg:col-span-6 relative hidden lg:block perspective-1000 animate-fade-in-up" style="animation-delay: 0.5s">
        <div class="relative w-[400px] mx-auto aspect-[3/4] rotate-y-12 rotate-z-3 shadow-2xl rounded-3xl overflow-hidden border-[6px] border-white bg-gray-900 transition-all duration-700 hover:rotate-0 hover:scale-105 group cursor-default">
          <div class="absolute inset-0 bg-gradient-to-br from-gray-800 to-black">
             <div class="absolute inset-0 opacity-30" style="background-image: radial-gradient(circle at 2px 2px, rgba(255,255,255,0.15) 1px, transparent 0); background-size: 20px 20px;"></div>
          </div>
          
          <div class="absolute inset-0 flex flex-col justify-end p-10">
            <div class="inline-flex self-start px-3 py-1 bg-white/10 backdrop-blur-md rounded-lg text-[10px] font-bold text-white mb-4 tracking-widest border border-white/10">
              NOW PLAYING
            </div>
            <div class="text-5xl font-black text-white mb-4 leading-none tracking-tighter">
              EMBER<br/><span class="text-transparent bg-clip-text bg-gradient-to-r from-ember to-orange-500">ORIGINAL</span>
            </div>
            <div class="text-white/70 text-sm font-medium mb-8 leading-relaxed">
              沉浸式观影体验，4K 原盘画质，极速秒播。
            </div>
            
            <div class="flex items-center gap-4">
              <div class="w-12 h-12 rounded-full bg-white flex items-center justify-center text-black shadow-lg shadow-white/20">
                <el-icon :size="24" class="ml-1"><VideoPlay /></el-icon>
              </div>
              <div class="flex-1">
                <div class="text-xs text-white/50 font-bold uppercase tracking-wider mb-1">Watching</div>
                <div class="h-1.5 bg-white/20 rounded-full overflow-hidden">
                  <div class="h-full bg-ember w-2/3"></div>
                </div>
              </div>
            </div>
          </div>
          
          <div class="absolute inset-0 bg-gradient-to-tr from-white/0 via-white/5 to-white/10 pointer-events-none"></div>
        </div>
        
        <div class="absolute top-1/4 -right-8 bg-white/80 backdrop-blur-xl p-4 rounded-2xl shadow-xl border border-white/50 animate-float" style="animation-delay: 1s;">
          <div class="flex items-center gap-4">
            <div class="w-12 h-12 bg-green-100 rounded-xl flex items-center justify-center text-green-600">
              <el-icon :size="24"><Connection /></el-icon>
            </div>
            <div>
              <div class="text-xs text-gray-500 font-bold uppercase tracking-wider">Status</div>
              <div class="text-base font-bold text-gray-900">Emby Connected</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>

<style scoped>
.perspective-1000 { perspective: 1000px; }
.rotate-y-12 { transform: rotateY(-12deg); }
.rotate-z-3 { transform: rotateZ(3deg); }

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10px); }
}
.animate-float { animation: float 6s ease-in-out infinite; }

.animate-fade-in-up {
  opacity: 0;
  animation: fadeInUp 0.8s ease-out forwards;
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
