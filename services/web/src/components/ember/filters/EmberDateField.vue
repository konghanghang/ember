<script setup lang="ts">
import type { Component } from 'vue'
import { Calendar } from '@element-plus/icons-vue'

defineOptions({
  inheritAttrs: false
})

const props = withDefaults(defineProps<{
  modelValue: string | null | undefined
  label: string
  placeholder?: string
  icon?: Component
}>(), {
  placeholder: '',
  icon: Calendar
})

const emit = defineEmits<{
  'update:modelValue': [value: string | null | undefined]
  change: [value: string | null | undefined]
}>()

const handleUpdate = (value: string | null | undefined) => {
  emit('update:modelValue', value)
}

const handleChange = (value: string | null | undefined) => {
  emit('change', value)
}
</script>

<template>
  <div class="space-y-1.5">
    <label class="text-xs font-semibold tracking-wide text-gray-500">{{ props.label }}</label>
    <div class="group relative w-full">
      <div class="pointer-events-none absolute inset-y-0 left-0 z-10 flex items-center pl-3">
        <el-icon class="text-gray-400 transition-colors group-focus-within:text-ember">
          <component :is="props.icon" />
        </el-icon>
      </div>
      <el-date-picker
        :model-value="props.modelValue"
        :placeholder="props.placeholder"
        class="w-full ember-filter-date"
        v-bind="$attrs"
        @update:model-value="handleUpdate"
        @change="handleChange"
      />
    </div>
  </div>
</template>
