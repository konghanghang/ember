<script setup lang="ts">
import { useSlots } from 'vue'

const props = withDefaults(defineProps<{
  wrapperClass?: string
  contentClass?: string
  actionsClass?: string
}>(), {
  wrapperClass: 'grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_auto]',
  contentClass: 'grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-4',
  actionsClass: 'flex items-end justify-end gap-2'
})

const slots = useSlots()
</script>

<template>
  <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
    <div :class="props.wrapperClass">
      <div :class="props.contentClass">
        <slot />
      </div>

      <div v-if="slots.actions" :class="props.actionsClass">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>
