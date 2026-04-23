<script setup lang="ts">
import type { Component } from 'vue'

defineOptions({
  inheritAttrs: false
})

const props = withDefaults(defineProps<{
  modelValue: string | number | boolean | null | undefined
  label: string
  placeholder?: string
  icon?: Component
}>(), {
  placeholder: '',
  icon: undefined
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | boolean | null | undefined]
  change: [value: string | number | boolean | null | undefined]
}>()

const handleUpdate = (value: string | number | boolean | null | undefined) => {
  emit('update:modelValue', value)
}

const handleChange = (value: string | number | boolean | null | undefined) => {
  emit('change', value)
}
</script>

<template>
  <div class="space-y-1.5">
    <label class="text-xs font-semibold tracking-wide text-gray-500">{{ props.label }}</label>
    <div class="relative w-full">
      <div v-if="props.icon" class="pointer-events-none absolute inset-y-0 left-0 z-10 flex items-center pl-3">
        <el-icon class="text-gray-400">
          <component :is="props.icon" />
        </el-icon>
      </div>
      <el-select
        :model-value="props.modelValue"
        :placeholder="props.placeholder"
        class="w-full ember-filter-select"
        :class="props.icon ? 'ember-filter-select-with-icon' : ''"
        v-bind="$attrs"
        @update:model-value="handleUpdate"
        @change="handleChange"
      >
        <slot />
      </el-select>
    </div>
  </div>
</template>
