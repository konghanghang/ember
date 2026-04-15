<script setup lang="ts">
import type { Component } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  icon?: Component
  tone?: 'neutral' | 'danger'
  compact?: boolean
}>(), {
  description: '',
  icon: undefined,
  tone: 'neutral',
  compact: false
})

const toneClassMap = {
  neutral: {
    wrapper: 'border-gray-200 bg-gray-50 text-gray-500',
    icon: 'bg-white text-gray-300'
  },
  danger: {
    wrapper: 'border-red-200 bg-red-50 text-red-700',
    icon: 'bg-red-100 text-red-300'
  }
} as const
</script>

<template>
  <article
    class="rounded-2xl border border-dashed text-center"
    :class="[
      toneClassMap[props.tone].wrapper,
      props.compact ? 'px-4 py-8' : 'px-6 py-10'
    ]"
  >
    <div
      v-if="props.icon"
      class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl"
      :class="toneClassMap[props.tone].icon"
    >
      <el-icon :size="22">
        <component :is="props.icon" />
      </el-icon>
    </div>

    <h3
      class="font-medium"
      :class="[props.icon ? 'mt-4' : '', props.compact ? 'text-sm' : 'text-lg']"
    >
      {{ props.title }}
    </h3>

    <p v-if="props.description" class="mt-2 text-sm leading-6">
      {{ props.description }}
    </p>

    <div v-if="$slots.actions" class="mt-6">
      <slot name="actions" />
    </div>
  </article>
</template>
