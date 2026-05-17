<script setup lang="ts">
const currentYear = new Date().getFullYear()
</script>

<template>
  <footer class="bg-gray-50 text-text-primary py-8 px-6 border-t border-gray-200">
    <div class="max-w-4xl mx-auto">
      <div class="flex flex-col md:flex-row justify-between items-center gap-4 text-xs text-text-secondary font-medium tracking-wide">
        <p>© {{ currentYear }} Ember Media. All rights reserved.</p>
        <div class="flex flex-wrap items-center justify-center gap-4 md:justify-end">
          <a href="#" class="hover:text-ember transition-colors">隐私政策</a>
          <a href="#" class="hover:text-ember transition-colors">服务条款</a>
        </div>
      </div>
    </div>
  </footer>
</template>
