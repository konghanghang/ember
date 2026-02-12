<script setup lang="ts">
import { useRouter } from 'vue-router'
import Navbar from '../components/home/Navbar.vue'
import Footer from '../components/home/Footer.vue'
import { VideoPlay, User, Connection, ArrowRight, Monitor } from '@element-plus/icons-vue'

const router = useRouter()
</script>

<template>
  <div class="min-h-screen bg-white text-gray-900 font-sans selection:bg-ember selection:text-white overflow-x-hidden flex flex-col">
    <Navbar />

    <!-- Hero Section: 更紧凑，更聚焦 -->
    <header class="relative pt-24 pb-16 lg:pt-32 lg:pb-24 px-6 flex-grow flex items-center">
      <div class="max-w-[1280px] mx-auto grid lg:grid-cols-12 gap-12 items-center w-full">
        
        <!-- 左侧文案 -->
        <div class="lg:col-span-6 relative z-10 text-center lg:text-left">
          <div class="inline-flex items-center gap-2 px-3 py-1 bg-red-50 border border-red-100 rounded-full text-xs font-bold tracking-widest uppercase mb-6 text-ember">
            <span class="w-2 h-2 rounded-full bg-ember animate-pulse"></span>
            Ember v1.0 正式上线
          </div>
          
          <h1 class="text-5xl md:text-7xl font-black tracking-tight leading-[1.1] mb-6">
            媒体管理，<br class="hidden lg:block"/>
            <span class="text-ember relative inline-block">
              从未如此优雅。
              <!-- 装饰下划线 -->
              <svg class="absolute -bottom-2 left-0 w-full h-3 text-ember/20" viewBox="0 0 100 10" preserveAspectRatio="none">
                <path d="M0 5 Q 50 10 100 5" stroke="currentColor" stroke-width="8" fill="none" />
              </svg>
            </span>
          </h1>
          
          <p class="text-lg text-gray-500 max-w-lg mx-auto lg:mx-0 mb-8 font-medium leading-relaxed">
            为您的 Emby 服务器打造的自动化管家。
            <br class="hidden md:block"/>
            从邀请注册到求片订阅，一切自动完成。
          </p>
          
          <div class="flex flex-col sm:flex-row gap-4 justify-center lg:justify-start">
            <button @click="router.push('/register')" class="group relative px-8 py-3.5 bg-ember text-white text-base font-bold tracking-wide rounded-lg transition-all hover:shadow-lg hover:shadow-ember/30 hover:-translate-y-0.5 overflow-hidden">
              <span class="relative z-10 flex items-center justify-center gap-2">
                立即体验
                <el-icon class="transition-transform group-hover:translate-x-1"><ArrowRight /></el-icon>
              </span>
              <div class="absolute inset-0 bg-black/10 translate-y-full group-hover:translate-y-0 transition-transform duration-300"></div>
            </button>
            
            <button @click="router.push('/login')" class="px-8 py-3.5 bg-white border border-gray-200 text-gray-700 text-base font-bold tracking-wide rounded-lg hover:border-gray-900 hover:text-gray-900 transition-all">
              登录
            </button>
          </div>
          
          <!-- 信任背书/小数据 -->
          <div class="mt-8 flex items-center justify-center lg:justify-start gap-6 text-sm text-gray-400 font-medium">
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

        <!-- 右侧视觉：更紧凑的票据堆叠 -->
        <div class="lg:col-span-6 relative hidden lg:block perspective-1000">
          <div class="relative w-[400px] mx-auto aspect-[3/4] rotate-y-12 rotate-z-3 shadow-2xl rounded-xl overflow-hidden border-4 border-white bg-gray-900 transition-all duration-700 hover:rotate-0 hover:scale-105 group cursor-default">
            <!-- 模拟电影海报界面 -->
            <div class="absolute inset-0 bg-gradient-to-br from-gray-800 to-black opacity-90"></div>
            <!-- 抽象海报内容 -->
            <div class="absolute top-0 inset-x-0 h-2/3 bg-gradient-to-b from-ember/20 to-transparent"></div>
            
            <div class="absolute inset-0 flex flex-col justify-end p-8">
              <div class="inline-flex self-start px-2 py-1 bg-white/10 backdrop-blur-sm rounded text-[10px] font-bold text-white mb-3 tracking-widest">
                正在热映
              </div>
              <div class="text-4xl font-black text-white mb-2 leading-none tracking-tight">
                EMBER<br/>ORIGINAL
              </div>
              <div class="text-white/60 text-xs font-medium mb-6 line-clamp-2">
                重新定义家庭媒体中心的交互体验。无需繁琐配置，开箱即用。
              </div>
              
              <!-- 播放进度条装饰 -->
              <div class="flex items-center gap-3 mb-2">
                <div class="w-8 h-8 rounded-full bg-white flex items-center justify-center text-black">
                  <el-icon class="ml-0.5"><VideoPlay /></el-icon>
                </div>
                <div class="flex-1 h-1 bg-white/20 rounded-full overflow-hidden">
                  <div class="h-full bg-ember w-2/3"></div>
                </div>
              </div>
            </div>
            
            <!-- 光泽效果 -->
            <div class="absolute inset-0 bg-gradient-to-tr from-white/0 via-white/5 to-white/10 pointer-events-none"></div>
          </div>
          
          <!-- 悬浮卡片 -->
          <div class="absolute top-1/4 -right-4 bg-white/90 backdrop-blur-md p-4 rounded-xl shadow-xl border border-white/50 animate-float" style="animation-delay: 1s;">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 bg-green-100 rounded-full flex items-center justify-center text-green-600">
                <el-icon><Connection /></el-icon>
              </div>
              <div>
                <div class="text-xs text-gray-500 font-bold uppercase">系统状态</div>
                <div class="text-sm font-bold text-gray-900">Emby 已连接</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </header>

    <!-- Marquee: 滚动条 -->
    <div class="bg-gray-50 border-y border-gray-100 py-4 overflow-hidden">
      <div class="flex whitespace-nowrap animate-marquee text-xs font-bold tracking-widest text-gray-400">
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 自动化用户管理</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> TMDB 深度集成</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 即时请求同步</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 安全访问控制</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 全平台兼容</span>
        <!-- 重复一次以实现无缝滚动 -->
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 自动化用户管理</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> TMDB 深度集成</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 即时请求同步</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 安全访问控制</span>
        <span class="mx-8 flex items-center gap-2"><span class="w-1.5 h-1.5 bg-ember rounded-full"></span> 全平台兼容</span>
      </div>
    </div>

    <!-- Features: 核心功能 (更紧凑的 Grid) -->
    <section class="py-20 px-6 bg-white">
      <div class="max-w-[1100px] mx-auto">
        <div class="text-center mb-12">
          <h2 class="text-sm font-bold text-ember tracking-widest uppercase mb-2">功能特性</h2>
          <h3 class="text-3xl font-black text-gray-900">为您打造的完整工作流</h3>
        </div>

        <div class="grid md:grid-cols-3 gap-6">
          <!-- Card 1 -->
          <div class="group relative p-8 rounded-2xl bg-gray-50 border border-gray-100 hover:border-red-100 hover:bg-white hover:shadow-xl hover:shadow-ember/5 transition-all duration-300">
            <div class="w-12 h-12 bg-white rounded-xl flex items-center justify-center mb-6 shadow-sm border border-gray-100 group-hover:scale-110 group-hover:border-red-100 transition-all duration-300">
              <el-icon class="text-2xl text-gray-700 group-hover:text-ember transition-colors"><User /></el-icon>
            </div>
            <h4 class="text-lg font-bold text-gray-900 mb-3">自助注册系统</h4>
            <p class="text-sm text-gray-500 leading-relaxed">
              告别手动发号。用户通过邀请码即可自助完成 Emby 账号注册，权限自动下发，省时省力。
            </p>
          </div>

          <!-- Card 2 -->
          <div class="group relative p-8 rounded-2xl bg-gray-50 border border-gray-100 hover:border-red-100 hover:bg-white hover:shadow-xl hover:shadow-ember/5 transition-all duration-300">
            <div class="w-12 h-12 bg-white rounded-xl flex items-center justify-center mb-6 shadow-sm border border-gray-100 group-hover:scale-110 group-hover:border-red-100 transition-all duration-300">
              <el-icon class="text-2xl text-gray-700 group-hover:text-ember transition-colors"><VideoPlay /></el-icon>
            </div>
            <h4 class="text-lg font-bold text-gray-900 mb-3">订阅与求片</h4>
            <p class="text-sm text-gray-500 leading-relaxed">
              内置 TMDB 搜索。用户想看什么直接搜，管理员一键批准，不再需要在群里手动统计需求。
            </p>
          </div>

          <!-- Card 3 -->
          <div class="group relative p-8 rounded-2xl bg-gray-50 border border-gray-100 hover:border-red-100 hover:bg-white hover:shadow-xl hover:shadow-ember/5 transition-all duration-300">
            <div class="w-12 h-12 bg-white rounded-xl flex items-center justify-center mb-6 shadow-sm border border-gray-100 group-hover:scale-110 group-hover:border-red-100 transition-all duration-300">
              <el-icon class="text-2xl text-gray-700 group-hover:text-ember transition-colors"><Monitor /></el-icon>
            </div>
            <h4 class="text-lg font-bold text-gray-900 mb-3">自动化同步</h4>
            <p class="text-sm text-gray-500 leading-relaxed">
              与 MoviePilot 等工具无缝集成。请求批准后自动触发下载任务，资源就绪后自动通知。
            </p>
          </div>
        </div>
      </div>
    </section>

    <Footer />
  </div>
</template>

<style scoped>
.perspective-1000 {
  perspective: 1000px;
}
.rotate-y-12 {
  transform: rotateY(-12deg);
}
.rotate-z-3 {
  transform: rotateZ(3deg);
}
@keyframes marquee {
  0% { transform: translateX(0); }
  100% { transform: translateX(-50%); }
}
.animate-marquee {
  animation: marquee 30s linear infinite;
}
@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-8px); }
}
.animate-float {
  animation: float 5s ease-in-out infinite;
}
</style>


