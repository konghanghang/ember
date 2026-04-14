<script setup lang="ts">
import type { Component } from 'vue'

type SegmentTab = {
  key: string
  label: string
  icon?: Component
}

const props = withDefaults(defineProps<{
  modelValue: string
  tabs: SegmentTab[]
  fullWidth?: boolean
}>(), {
  fullWidth: true
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  change: [value: string]
}>()

const handleSelect = (value: string) => {
  if (value === props.modelValue) return
  emit('update:modelValue', value)
  emit('change', value)
}
</script>

<template>
  <div :class="['inline-flex rounded-2xl bg-slate-100 p-1', props.fullWidth ? 'w-full lg:w-auto' : 'w-auto']">
    <button
      v-for="tab in props.tabs"
      :key="tab.key"
      type="button"
      class="flex items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors"
      :class="[
        props.fullWidth ? 'flex-1 lg:flex-none' : '',
        props.modelValue === tab.key ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500 hover:text-slate-700'
      ]"
      @click="handleSelect(tab.key)"
    >
      <el-icon v-if="tab.icon"><component :is="tab.icon" /></el-icon>
      <span>{{ tab.label }}</span>
    </button>
  </div>
</template>
