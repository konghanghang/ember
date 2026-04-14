<script setup lang="ts">
const props = defineProps<{
  title: string
  value: string | number
  detail?: string
}>()
</script>

<template>
  <article class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
    <p class="text-sm text-gray-500">{{ props.title }}</p>
    <p class="mt-3 text-3xl font-bold text-gray-900">{{ props.value }}</p>
    <p v-if="props.detail" class="mt-2 text-xs text-gray-500">{{ props.detail }}</p>
    <slot />
  </article>
</template>
