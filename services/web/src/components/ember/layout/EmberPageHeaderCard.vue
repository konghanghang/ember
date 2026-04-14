<script setup lang="ts">
import { useSlots } from 'vue'

const props = defineProps<{
  title: string
  description?: string
}>()

const slots = useSlots()
</script>

<template>
  <section class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm">
    <div :class="slots.actions ? 'flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between' : ''">
      <div>
        <h1 class="flex items-center gap-2 text-2xl font-bold text-gray-900">
          <span>{{ props.title }}</span>
          <slot name="titleSuffix" />
        </h1>
        <p v-if="props.description" class="mt-1 text-sm text-gray-500">
          {{ props.description }}
        </p>
      </div>

      <div v-if="slots.actions" class="self-start">
        <slot name="actions" />
      </div>
    </div>

    <slot />
  </section>
</template>
