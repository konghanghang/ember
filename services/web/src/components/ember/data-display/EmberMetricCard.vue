<script setup lang="ts">
import type { Tone } from '@/components/ember/tokens'

const props = defineProps<{
  title: string
  value: string | number
  detail?: string
  valueClass?: string
  tone?: Tone
}>()

const toneClassMap = {
  neutral: {
    wrapper: 'border-gray-100 bg-white',
    title: 'text-gray-500'
  },
  info: {
    wrapper: 'border-sky-100 bg-sky-50/40',
    title: 'text-sky-700'
  },
  success: {
    wrapper: 'border-emerald-100 bg-emerald-50/40',
    title: 'text-emerald-700'
  },
  warning: {
    wrapper: 'border-amber-100 bg-amber-50/40',
    title: 'text-amber-700'
  },
  danger: {
    wrapper: 'border-red-100 bg-red-50/40',
    title: 'text-red-700'
  }
} as const
</script>

<template>
  <article class="rounded-2xl border p-5 shadow-sm" :class="toneClassMap[props.tone || 'neutral'].wrapper">
    <p class="text-sm" :class="toneClassMap[props.tone || 'neutral'].title">{{ props.title }}</p>
    <p :class="props.valueClass || 'mt-3 text-3xl font-bold text-gray-900'">{{ props.value }}</p>
    <p v-if="props.detail" class="mt-2 text-xs text-gray-500">{{ props.detail }}</p>
    <slot />
  </article>
</template>
