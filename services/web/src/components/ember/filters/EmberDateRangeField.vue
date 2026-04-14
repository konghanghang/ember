<script setup lang="ts">
import type { Component } from 'vue'
import { Calendar } from '@element-plus/icons-vue'

defineOptions({
  inheritAttrs: false
})

const props = withDefaults(defineProps<{
  modelValue: [string, string] | [] | null | undefined
  label: string
  icon?: Component
}>(), {
  icon: Calendar
})

const emit = defineEmits<{
  'update:modelValue': [value: [string, string] | [] | null | undefined]
  change: [value: [string, string] | [] | null | undefined]
  calendarChange: [value: unknown]
}>()

const handleUpdate = (value: [string, string] | [] | null | undefined) => {
  emit('update:modelValue', value)
}

const handleChange = (value: [string, string] | [] | null | undefined) => {
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
        class="w-full ember-filter-date-range"
        v-bind="$attrs"
        @update:model-value="handleUpdate"
        @change="handleChange"
        @calendar-change="emit('calendarChange', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
:deep(.ember-filter-date-range.el-date-editor),
:deep(.ember-filter-date-range .el-range-editor.el-input__wrapper),
:deep(.ember-filter-date-range.el-range-editor.el-input__wrapper) {
  height: 42px !important;
  min-height: 42px !important;
  border-radius: 0.75rem !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  background-color: #f9fafb !important;
}

:deep(.ember-filter-date-range .el-input__wrapper),
:deep(.ember-filter-date-range.el-input__wrapper) {
  overflow: hidden;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
  padding-left: 2.5rem !important;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  background-color: #f9fafb !important;
}

:deep(.ember-filter-date-range:hover),
:deep(.ember-filter-date-range:hover .el-input__wrapper) {
  background-color: #ffffff !important;
}

:deep(.ember-filter-date-range.is-active),
:deep(.ember-filter-date-range.is-active .el-input__wrapper) {
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
  background-color: #ffffff !important;
}

:deep(.ember-filter-date-range .el-range-input) {
  width: 11rem;
  min-width: 11rem;
  background-color: transparent;
  font-size: 0.875rem;
}

:deep(.ember-filter-date-range .el-range-separator) {
  min-width: 1.5rem;
  justify-content: center;
  color: #6b7280;
}

:deep(.ember-filter-date-range .el-range__icon) {
  opacity: 0;
  width: 0;
  margin: 0;
}

:deep(.ember-filter-date-range .el-range__close-icon) {
  display: none !important;
}
</style>
