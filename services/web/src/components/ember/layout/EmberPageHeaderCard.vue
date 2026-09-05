<script setup lang="ts">
import { useSlots } from 'vue'

const props = defineProps<{
  title: string
  description?: string
  hideTitle?: boolean
}>()

const slots = useSlots()
</script>

<template>
  <section
    class="rounded-2xl border border-gray-100 bg-white p-6 shadow-sm"
    :aria-label="props.hideTitle ? props.title : undefined"
  >
    <div
      v-if="!props.hideTitle || slots.titleSuffix || slots.actions"
      :class="[
        'flex flex-col gap-4',
        props.hideTitle
          ? 'sm:flex-row sm:items-center sm:justify-between'
          : slots.actions
            ? 'lg:flex-row lg:items-start lg:justify-between'
            : ''
      ]"
    >
      <div v-if="!props.hideTitle">
        <h1 class="flex items-center gap-2 text-2xl font-bold text-gray-900">
          <span>{{ props.title }}</span>
          <slot name="titleSuffix" />
        </h1>
        <p v-if="props.description" class="mt-1 text-sm text-gray-500">
          {{ props.description }}
        </p>
      </div>

      <div v-else-if="slots.titleSuffix" class="flex flex-wrap items-center gap-2">
        <slot name="titleSuffix" />
      </div>

      <div
        v-if="slots.actions"
        :class="props.hideTitle ? 'self-end sm:ml-auto' : 'self-stretch lg:self-start'"
      >
        <slot name="actions" />
      </div>
    </div>

    <slot />
  </section>
</template>
