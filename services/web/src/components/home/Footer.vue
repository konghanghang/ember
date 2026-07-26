<script setup lang="ts">
const currentYear = new Date().getFullYear()
</script>

<template>
  <footer class="bg-gray-50 text-text-primary py-8 px-6 border-t border-gray-200">
    <div class="max-w-7xl mx-auto">
      <div class="flex flex-col md:flex-row justify-between items-center gap-4 text-xs text-text-secondary font-medium tracking-wide">
        <p>© {{ currentYear }} Ember Media. All rights reserved.</p>
      </div>
    </div>
  </footer>
</template>
