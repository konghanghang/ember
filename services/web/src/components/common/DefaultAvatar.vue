<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  name?: string | null
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'hero'
  shape?: 'full' | 'xl' | '2xl'
}>(), {
  name: '',
  size: 'md',
  shape: '2xl'
})

const palette = [
  'bg-rose-50 text-rose-700 ring-rose-100',
  'bg-amber-50 text-amber-700 ring-amber-100',
  'bg-emerald-50 text-emerald-700 ring-emerald-100',
  'bg-sky-50 text-sky-700 ring-sky-100',
  'bg-violet-50 text-violet-700 ring-violet-100',
  'bg-fuchsia-50 text-fuchsia-700 ring-fuchsia-100',
  'bg-orange-50 text-orange-700 ring-orange-100',
  'bg-cyan-50 text-cyan-700 ring-cyan-100',
] as const

const normalizedName = computed(() => (props.name || '').trim())

const initial = computed(() => {
  const firstChar = normalizedName.value.slice(0, 1)
  return (firstChar || 'U').toUpperCase()
})

const paletteClass = computed(() => {
  let hash = 0
  for (const char of normalizedName.value || 'U') {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0
  }
  return palette[hash % palette.length]
})

const sizeClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-8 w-8 text-sm'
    case 'lg':
      return 'h-12 w-12 text-base'
    case 'xl':
      return 'h-16 w-16 text-2xl'
    case 'hero':
      return 'h-20 w-20 text-3xl'
    default:
      return 'h-10 w-10 text-sm'
  }
})

const shapeClass = computed(() => {
  switch (props.shape) {
    case 'full':
      return 'rounded-full'
    case 'xl':
      return 'rounded-xl'
    default:
      return 'rounded-2xl'
  }
})
</script>

<template>
  <div
    class="inline-flex shrink-0 items-center justify-center font-semibold leading-none ring-1 select-none"
    :class="[paletteClass, sizeClass, shapeClass]"
    :aria-label="`默认头像 ${initial}`"
  >
    {{ initial }}
  </div>
</template>
