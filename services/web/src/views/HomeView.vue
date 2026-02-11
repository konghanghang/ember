<script setup lang="ts">
import Navbar from '@/components/home/Navbar.vue'
import HeroSection from '@/components/home/HeroSection.vue'
import FeaturesSection from '@/components/home/FeaturesSection.vue'
import PricingSection from '@/components/home/PricingSection.vue'
import FAQSection from '@/components/home/FAQSection.vue'
import Footer from '@/components/home/Footer.vue'
</script>

<template>
  <div class="min-h-screen">
    <Navbar />
    <HeroSection />
    <FeaturesSection />
    <PricingSection />
    <FAQSection />
    <Footer />
  </div>
</template>
