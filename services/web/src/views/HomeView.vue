<script setup lang="ts">
import Navbar from '../components/home/Navbar.vue'
import HeroSection from '../components/home/HeroSection.vue'
import MarqueeSection from '../components/home/MarqueeSection.vue'
import FeaturesSection from '../components/home/FeaturesSection.vue'
import Footer from '../components/home/Footer.vue'
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 font-sans selection:bg-ember selection:text-white overflow-x-hidden flex flex-col">
    <Navbar />
    
    <main class="flex-grow">
      <HeroSection />
      <MarqueeSection />
      <FeaturesSection />
    </main>

    <Footer />
  </div>
</template>
